package pii

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// backfillFields converts the human-readable columns that predate field encryption: it seals them
// and fills the keyed hashes the database now compares instead. Safe to run on every start; it only
// touches rows that are not converted yet.
func (b *Box) backfillFields(ctx context.Context, pool *pgxpool.Pool, counts map[string]int) error {
	steps := []func(context.Context, *pgxpool.Pool, map[string]int) error{
		b.backfillEnvironments, b.backfillSecrets, b.backfillEnvRoles, b.backfillDynamic, b.convertColumns, b.unsealEnvRoles,
	}
	for _, step := range steps {
		if err := step(ctx, pool, counts); err != nil {
			return err
		}
	}
	return nil
}

type fieldRow struct{ a, b, c, d, e string }

func collect(ctx context.Context, pool *pgxpool.Pool, sql string, width int) ([]fieldRow, error) {
	rows, err := pool.Query(ctx, sql)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []fieldRow
	for rows.Next() {
		var x fieldRow
		dst := []any{&x.a, &x.b, &x.c, &x.d, &x.e}[:width]
		if err := rows.Scan(dst...); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (b *Box) backfillEnvironments(ctx context.Context, pool *pgxpool.Pool, counts map[string]int) error {
	for {
		batch, err := collect(ctx, pool, `SELECT id::text, name, slug FROM environments WHERE slug_hash IS NULL LIMIT 500`, 3)
		if err != nil || len(batch) == 0 {
			return err
		}
		for _, x := range batch {
			if _, err := pool.Exec(ctx, `UPDATE environments SET name=$2, slug=$3, slug_hash=$4 WHERE id::text=$1`,
				x.a, b.SealName("environments.name", x.b), b.SealName("environments.slug", x.c), b.Hash("environments.slug", x.c)); err != nil {
				return fmt.Errorf("environments: %w", err)
			}
			counts["environments"]++
		}
	}
}

func (b *Box) backfillSecrets(ctx context.Context, pool *pgxpool.Pool, counts map[string]int) error {
	for {
		batch, err := collect(ctx, pool, `SELECT id::text, path, key, comment, tags FROM secrets WHERE lookup_hash IS NULL LIMIT 300`, 5)
		if err != nil || len(batch) == 0 {
			return err
		}
		for _, x := range batch {
			tags := x.e
			if tags == "" {
				tags = "[]"
			}
			if _, err := pool.Exec(ctx, `UPDATE secrets SET path=$2, key=$3, comment=$4, tags=$5, path_hash=$6, lookup_hash=$7 WHERE id::text=$1`,
				x.a, b.SealName("secrets.path", x.b), b.SealName("secrets.key", x.c), b.SealName("secrets.comment", x.d), b.SealName("secrets.tags", tags),
				b.Hash("secrets.path", x.b), b.Hash("secrets.lookup", x.b+"\x00"+x.c)); err != nil {
				return fmt.Errorf("secrets: %w", err)
			}
			counts["secrets"]++
		}
	}
}

func (b *Box) backfillEnvRoles(ctx context.Context, pool *pgxpool.Pool, counts map[string]int) error {
	for {
		batch, err := collect(ctx, pool, `SELECT org_id::text, user_id::text, env_slug FROM member_env_roles WHERE env_slug_hash IS NULL LIMIT 500`, 3)
		if err != nil || len(batch) == 0 {
			return err
		}
		for _, x := range batch {
			if _, err := pool.Exec(ctx, `UPDATE member_env_roles SET env_slug=$4, env_slug_hash=$5 WHERE org_id::text=$1 AND user_id::text=$2 AND env_slug=$3`,
				x.a, x.b, x.c, b.SealName("member_env_roles.slug", x.c), b.Hash("environments.slug", x.c)); err != nil {
				return fmt.Errorf("member_env_roles: %w", err)
			}
			counts["member_env_roles"]++
		}
	}
}

func (b *Box) backfillDynamic(ctx context.Context, pool *pgxpool.Pool, counts map[string]int) error {
	for {
		batch, err := collect(ctx, pool, `SELECT id::text, name FROM dynamic_configs WHERE name_hash IS NULL LIMIT 500`, 2)
		if err != nil || len(batch) == 0 {
			return err
		}
		for _, x := range batch {
			if _, err := pool.Exec(ctx, `UPDATE dynamic_configs SET name=$2, name_hash=$3 WHERE id::text=$1`,
				x.a, b.SealName("dynamic_configs.name", x.b), b.Hash("dynamic_configs.name", x.b)); err != nil {
				return fmt.Errorf("dynamic_configs: %w", err)
			}
			counts["dynamic_configs"]++
		}
	}
}

// convertColumns brings every text column in line with the policy: values of sealed fields are
// encrypted, values of readable fields that an earlier version encrypted are decrypted again.
func (b *Box) convertColumns(ctx context.Context, pool *pgxpool.Pool, counts map[string]int) error {
	for _, c := range []struct{ table, col, field string }{
		{"orgs", "name", "orgs.name"}, {"projects", "name", "projects.name"},
		{"environments", "name", "environments.name"}, {"environments", "slug", "environments.slug"},
		{"secrets", "path", "secrets.path"}, {"secrets", "key", "secrets.key"}, {"secrets", "comment", "secrets.comment"}, {"secrets", "tags", "secrets.tags"},
		{"service_tokens", "name", "service_tokens.name"}, {"service_tokens", "token_prefix", "service_tokens.prefix"},
		{"service_tokens", "path_prefix", "service_tokens.path"}, {"service_tokens", "allowed_cidrs", "service_tokens.cidrs"},
		{"personal_tokens", "name", "personal_tokens.name"}, {"personal_tokens", "token_prefix", "personal_tokens.prefix"},
		{"webhooks", "name", "webhooks.name"}, {"webhooks", "url", "webhooks.url"}, {"webhooks", "events", "webhooks.events"},
		{"webhook_deliveries", "action", "webhook_deliveries.action"}, {"webhook_deliveries", "error", "webhook_deliveries.error"},
		{"integrations", "name", "integrations.name"}, {"integrations", "path", "integrations.path"}, {"integrations", "last_error", "integrations.last_error"},
		{"dynamic_configs", "name", "dynamic_configs.name"},
		{"dynamic_leases", "username", "dynamic_leases.username"}, {"dynamic_leases", "revoke_error", "dynamic_leases.revoke_error"},
		{"invites", "env_roles", "invites.env_roles"}, {"audit_logs", "metadata", "audit.metadata"},
		{"change_requests", "comment", "change_requests.comment"}, {"change_requests", "note", "change_requests.note"},
	} {
		var n int
		var err error
		field := c.field
		if Encrypted(field) {
			n, err = b.sealText(ctx, pool, c.table, "id", c.col, func(v string) string { return b.SealName(field, v) })
		} else {
			n, err = b.unsealText(ctx, pool, c.table, c.col, field)
		}
		if err != nil {
			return fmt.Errorf("%s.%s: %w", c.table, c.col, err)
		}
		if n > 0 {
			counts[c.table+"."+c.col] = n
		}
	}
	return nil
}

// unsealText decrypts every sealed value of a column whose field the policy keeps readable.
// Values that cannot be decrypted are left alone.
func (b *Box) unsealText(ctx context.Context, pool *pgxpool.Pool, table, col, field string) (int, error) {
	batch, err := collect(ctx, pool, fmt.Sprintf(`SELECT id::text, %s FROM %s WHERE %s LIKE '%s%%'`, col, table, col, Prefix), 2)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, x := range batch {
		plain, err := b.Open(field, x.b)
		if err != nil {
			continue
		}
		if _, err := pool.Exec(ctx, fmt.Sprintf(`UPDATE %s SET %s=$2 WHERE id::text=$1`, table, col), x.a, plain); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// unsealEnvRoles does the same for member_env_roles, whose key is (org, user, slug) rather than an id.
func (b *Box) unsealEnvRoles(ctx context.Context, pool *pgxpool.Pool, counts map[string]int) error {
	if Encrypted("member_env_roles.slug") {
		return nil
	}
	batch, err := collect(ctx, pool, fmt.Sprintf(`SELECT org_id::text, user_id::text, env_slug FROM member_env_roles WHERE env_slug LIKE '%s%%'`, Prefix), 3)
	if err != nil {
		return err
	}
	for _, x := range batch {
		plain, err := b.Open("member_env_roles.slug", x.c)
		if err != nil {
			continue
		}
		if _, err := pool.Exec(ctx, `UPDATE member_env_roles SET env_slug=$4 WHERE org_id::text=$1 AND user_id::text=$2 AND env_slug=$3`, x.a, x.b, x.c, plain); err != nil {
			return fmt.Errorf("member_env_roles: %w", err)
		}
		counts["member_env_roles (readable)"]++
	}
	return nil
}
