// Package pii encrypts personal data (emails, names, actor labels) at the
// application layer so a database dump reveals no identities.
//
//	Seal/Open   AES-256-GCM, random nonce, field name bound as additional data
//	Index       HMAC-SHA256 "blind index": equal emails give equal hashes, so you can
//	            look a user up or enforce uniqueness without storing the email in clear
//
// Both keys are random, stored wrapped by the KeyProvider (OpenBao or MASTER_KEY).
package pii

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"vaultly/internal/crypto"
)

const Prefix = "enc:v1:"

type Box struct{ enc, idx []byte }

func New(enc, idx []byte) *Box { return &Box{enc: enc, idx: idx} }

func Sealed(s string) bool { return strings.HasPrefix(s, Prefix) }

func (b *Box) Seal(field, plain string) string {
	ct, err := crypto.Encrypt(b.enc, []byte(plain), []byte(field))
	if err != nil {
		panic(fmt.Sprintf("pii: encrypt: %v", err)) // only possible with a bad key length
	}
	return Prefix + base64.RawStdEncoding.EncodeToString(ct)
}

func (b *Box) Open(field, s string) (string, error) {
	if !Sealed(s) {
		return "", errors.New("value is not sealed")
	}
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(s, Prefix))
	if err != nil {
		return "", err
	}
	pt, err := crypto.Decrypt(b.enc, raw, []byte(field))
	return string(pt), err
}

// Index is the blind index of an email-like value (case and surrounding spaces ignored).
func (b *Box) Index(s string) []byte {
	m := hmac.New(sha256.New, b.idx)
	m.Write([]byte(strings.ToLower(strings.TrimSpace(s))))
	return m.Sum(nil)
}

// Label seals actor labels and resources that contain an email address; token
// names and other labels pass through. It is idempotent.
func (b *Box) Label(s string) string {
	if s == "" || Sealed(s) || !strings.Contains(s, "@") {
		return s
	}
	return b.Seal("label", s)
}

// OpenLabel reverses Label. Plaintext (legacy) values pass through; an unreadable
// value is shown as a placeholder instead of failing the whole page.
func (b *Box) OpenLabel(s string) string {
	if !Sealed(s) {
		return s
	}
	if p, err := b.Open("label", s); err == nil {
		return p
	}
	return "[unreadable]"
}

// sealedFields is the policy: the columns stored encrypted. Everything else is stored as readable text.
//
// Encrypted: personal data about people (workspace names, which default to a person's name), and
// anything credential-adjacent or revealing about a customer's systems: what a secret is called and
// why it exists, token prefixes, IP allowlists, webhook URLs (some contain tokens) and the error
// messages that quote them. Emails, names and audit actors/IPs are sealed through Seal/Label.
//
// Readable: labels a team browses and the database orders by (project, environment and folder names,
// tags, token and integration names, role lists, audit metadata). Moving a field between the two is a
// one-line change here; the startup conversion (Backfill) re-seals or un-seals existing rows.
var sealedFields = map[string]bool{
	"orgs.name":                   true,
	"secrets.key":                 true,
	"secrets.comment":             true,
	"change_requests.comment":     true, // same text as secrets.comment, proposed for review
	"change_requests.note":        true, // a reviewer's note can quote the value or the reason
	"service_tokens.prefix":       true,
	"service_tokens.cidrs":        true,
	"personal_tokens.prefix":      true,
	"webhooks.url":                true,
	"webhook_deliveries.error":    true,
	"integrations.last_error":     true,
	"dynamic_leases.revoke_error": true,
}

// Encrypted reports whether the policy seals this field.
func Encrypted(field string) bool { return sealedFields[field] }

// SealName stores s encrypted if the policy says so, and as-is otherwise. It is idempotent, and the
// field name ("orgs.name") is bound into the ciphertext.
func (b *Box) SealName(field, s string) string {
	if s == "" || Sealed(s) || !sealedFields[field] {
		return s
	}
	return b.Seal(field, s)
}

// OpenName reverses SealName. Plaintext (legacy) values pass through; an unreadable value is
// shown as a placeholder instead of failing the whole page.
func (b *Box) OpenName(field, s string) string {
	if !Sealed(s) {
		return s
	}
	if p, err := b.Open(field, s); err == nil {
		return p
	}
	return "[unreadable]"
}

// Hash is a keyed, case-sensitive lookup value for equality checks on encrypted columns
// (a secret's key + path, an environment's slug). Identical inputs give identical hashes, so it
// reveals only "these two values are equal"; the field name keeps different columns apart.
func (b *Box) Hash(field, s string) []byte {
	m := hmac.New(sha256.New, b.idx)
	m.Write([]byte(field + "\x00" + s))
	return m.Sum(nil)
}

// SealJSON encrypts any value (lists, maps) as JSON. OpenJSON reverses it; legacy plaintext JSON passes through.
func (b *Box) SealJSON(field string, v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("pii: marshal: %v", err))
	}
	return b.SealName(field, string(raw))
}

func (b *Box) OpenJSON(field, s string, out any) error {
	if s == "" {
		return nil
	}
	if Sealed(s) {
		plain, err := b.Open(field, s)
		if err != nil {
			return err
		}
		s = plain
	}
	return json.Unmarshal([]byte(s), out)
}

// LabelAll is Label for columns where every value is sealed, not only those containing an address
// (audit actors and resources). OpenLabel reads both.
func (b *Box) LabelAll(s string) string {
	if s == "" || Sealed(s) {
		return s
	}
	return b.Seal("label", s)
}

// Load returns the Box for this deployment, creating its two keys on first start.
func Load(ctx context.Context, pool *pgxpool.Pool, kp crypto.KeyProvider) (*Box, error) {
	keys := map[string][]byte{}
	for _, name := range []string{"pii-enc", "pii-index"} {
		var wrapped []byte
		err := pool.QueryRow(ctx, `SELECT wrapped FROM app_keys WHERE name=$1`, name).Scan(&wrapped)
		if err != nil {
			_, w, nerr := kp.NewDEK(ctx)
			if nerr != nil {
				return nil, nerr
			}
			if _, err := pool.Exec(ctx, `INSERT INTO app_keys (name, wrapped) VALUES ($1,$2) ON CONFLICT (name) DO NOTHING`, name, w); err != nil {
				return nil, err
			}
			if err := pool.QueryRow(ctx, `SELECT wrapped FROM app_keys WHERE name=$1`, name).Scan(&wrapped); err != nil {
				return nil, err
			}
		}
		plain, err := kp.UnwrapDEK(ctx, wrapped)
		if err != nil {
			return nil, fmt.Errorf("unwrap %s: %w", name, err)
		}
		keys[name] = plain
	}
	return New(keys["pii-enc"], keys["pii-index"]), nil
}

// RewrapKeys re-protects the stored PII keys under the provider's current key.
func RewrapKeys(ctx context.Context, pool *pgxpool.Pool, kp crypto.KeyProvider) (int, error) {
	rows, err := pool.Query(ctx, `SELECT name, wrapped FROM app_keys`)
	if err != nil {
		return 0, err
	}
	type row struct {
		name string
		w    []byte
	}
	var all []row
	for rows.Next() {
		var r row
		if rows.Scan(&r.name, &r.w) == nil {
			all = append(all, r)
		}
	}
	rows.Close()
	for _, r := range all {
		nw, err := kp.Rewrap(ctx, r.w)
		if err != nil {
			return 0, fmt.Errorf("%s: %w", r.name, err)
		}
		if _, err := pool.Exec(ctx, `UPDATE app_keys SET wrapped=$2 WHERE name=$1 AND wrapped=$3`, r.name, nw, r.w); err != nil {
			return 0, err
		}
	}
	return len(all), nil
}

// Backfill encrypts any personal data still stored in plaintext. It is safe to run
// on every start: it only touches rows that are not yet sealed.
func (b *Box) Backfill(ctx context.Context, pool *pgxpool.Pool) error {
	counts := map[string]int{}

	for {
		rows, err := pool.Query(ctx, `SELECT id, coalesce(email,''), coalesce(name,'') FROM users WHERE email_enc IS NULL LIMIT 200`)
		if err != nil {
			return err
		}
		type u struct{ id, email, name string }
		var batch []u
		for rows.Next() {
			var x u
			if rows.Scan(&x.id, &x.email, &x.name) == nil {
				batch = append(batch, x)
			}
		}
		rows.Close()
		if len(batch) == 0 {
			break
		}
		for _, x := range batch {
			if _, err := pool.Exec(ctx, `UPDATE users SET email_enc=$2, email_hash=$3, name_enc=$4, email=NULL, name=NULL WHERE id=$1 AND email_enc IS NULL`,
				x.id, b.Seal("users.email", x.email), b.Index(x.email), b.Seal("users.name", x.name)); err != nil {
				return err
			}
			counts["users"]++
		}
	}

	for {
		rows, err := pool.Query(ctx, `SELECT id, coalesce(email,'') FROM invites WHERE email_enc IS NULL LIMIT 200`)
		if err != nil {
			return err
		}
		type i struct{ id, email string }
		var batch []i
		for rows.Next() {
			var x i
			if rows.Scan(&x.id, &x.email) == nil {
				batch = append(batch, x)
			}
		}
		rows.Close()
		if len(batch) == 0 {
			break
		}
		for _, x := range batch {
			if _, err := pool.Exec(ctx, `UPDATE invites SET email_enc=$2, email_hash=$3, email=NULL WHERE id=$1 AND email_enc IS NULL`,
				x.id, b.Seal("invites.email", x.email), b.Index(x.email)); err != nil {
				return err
			}
			counts["invites"]++
		}
	}

	// Free-text columns that may hold an email address (who did it).
	for _, c := range []struct{ table, pk, col string }{
		{"secrets", "id", "updated_by"}, {"secret_versions", "id", "actor"}, {"service_tokens", "id", "created_by"},
		{"invites", "id", "invited_by"}, {"dynamic_leases", "id", "created_by"},
		{"audit_logs", "id", "actor_label"}, {"audit_logs", "id", "resource"},
	} {
		n, err := b.sealColumn(ctx, pool, c.table, c.pk, c.col)
		if err != nil {
			return fmt.Errorf("%s.%s: %w", c.table, c.col, err)
		}
		if n > 0 {
			counts[c.table+"."+c.col] = n
		}
	}
	for _, c := range []struct{ table, pk, col string }{{"audit_logs", "id", "actor_label"}, {"audit_logs", "id", "resource"}, {"audit_logs", "id", "ip"}} {
		n, err := b.sealText(ctx, pool, c.table, c.pk, c.col, b.LabelAll)
		if err != nil {
			return fmt.Errorf("%s.%s: %w", c.table, c.col, err)
		}
		if n > 0 {
			counts[c.table+"."+c.col+" (all)"] = n
		}
	}
	if err := b.backfillFields(ctx, pool, counts); err != nil {
		return err
	}
	if len(counts) > 0 {
		log.Printf("pii: converted existing rows: %v", counts)
	}
	return nil
}

// sealColumn's identifiers are constants from Backfill, never user input.
func (b *Box) sealColumn(ctx context.Context, pool *pgxpool.Pool, table, pk, col string) (int, error) {
	total := 0
	for {
		rows, err := pool.Query(ctx, fmt.Sprintf(`SELECT %s::text, %s FROM %s WHERE %s LIKE '%%@%%' AND %s NOT LIKE '%s%%' LIMIT 500`, pk, col, table, col, col, Prefix))
		if err != nil {
			return total, err
		}
		type r struct{ id, v string }
		var batch []r
		for rows.Next() {
			var x r
			if rows.Scan(&x.id, &x.v) == nil {
				batch = append(batch, x)
			}
		}
		rows.Close()
		if len(batch) == 0 {
			return total, nil
		}
		for _, x := range batch {
			if _, err := pool.Exec(ctx, fmt.Sprintf(`UPDATE %s SET %s=$2 WHERE %s::text=$1`, table, col, pk), x.id, b.Label(x.v)); err != nil {
				return total, err
			}
			total++
		}
	}
}

// sealText seals every not-yet-sealed value of a column using seal. Identifiers are constants from Backfill.
func (b *Box) sealText(ctx context.Context, pool *pgxpool.Pool, table, pk, col string, seal func(string) string) (int, error) {
	total := 0
	for {
		rows, err := pool.Query(ctx, fmt.Sprintf(`SELECT %s::text, %s FROM %s WHERE %s <> '' AND %s NOT LIKE '%s%%' LIMIT 500`, pk, col, table, col, col, Prefix))
		if err != nil {
			return total, err
		}
		type r struct{ id, v string }
		var batch []r
		for rows.Next() {
			var x r
			if rows.Scan(&x.id, &x.v) == nil {
				batch = append(batch, x)
			}
		}
		rows.Close()
		if len(batch) == 0 {
			return total, nil
		}
		for _, x := range batch {
			if _, err := pool.Exec(ctx, fmt.Sprintf(`UPDATE %s SET %s=$2 WHERE %s::text=$1`, table, col, pk), x.id, seal(x.v)); err != nil {
				return total, err
			}
			total++
		}
	}
}
