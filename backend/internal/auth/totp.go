package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

// TOTP per RFC 6238: HMAC-SHA1, 30-second steps, 6 digits — what every authenticator app expects.
const (
	totpStep   = 30
	totpDigits = 6
	totpSkew   = 1 // accept one step either side for clock drift
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewTOTPSecret returns a random 160-bit secret, base32-encoded for authenticator apps.
func NewTOTPSecret() string {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		panic("totp: no randomness available: " + err.Error())
	}
	return b32.EncodeToString(b)
}

// TOTPStep is the time step a moment falls in.
func TOTPStep(t time.Time) int64 { return t.Unix() / totpStep }

// TOTPCode is the code for a given step.
func TOTPCode(secret string, step int64) (string, error) {
	key, err := b32.DecodeString(strings.ToUpper(secret))
	if err != nil {
		return "", err
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	m := hmac.New(sha1.New, key)
	m.Write(msg[:])
	sum := m.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	n := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", totpDigits, n%1_000_000), nil
}

// VerifyTOTP checks a code against the steps around now and returns the step it matched.
// Callers must reject any step <= the last one they accepted, so a code can't be replayed.
func VerifyTOTP(secret, code string, now time.Time) (step int64, ok bool) {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != totpDigits {
		return 0, false
	}
	cur := TOTPStep(now)
	for d := int64(-totpSkew); d <= totpSkew; d++ {
		want, err := TOTPCode(secret, cur+d)
		if err != nil {
			return 0, false
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return cur + d, true
		}
	}
	return 0, false
}

// TOTPURL is the otpauth:// link authenticator apps import.
func TOTPURL(secret, account string) string {
	v := url.Values{}
	v.Set("secret", secret)
	v.Set("issuer", "Vaultly")
	v.Set("algorithm", "SHA1")
	v.Set("digits", fmt.Sprint(totpDigits))
	v.Set("period", fmt.Sprint(totpStep))
	return "otpauth://totp/" + url.PathEscape("Vaultly:"+account) + "?" + v.Encode()
}

// TOTPQR renders the otpauth link as a PNG data URL, so the browser needs no QR library.
func TOTPQR(otpURL string) (string, error) {
	png, err := qrcode.Encode(otpURL, qrcode.Medium, 256)
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), nil
}

// NewRecoveryCodes returns n codes like "k7m2q-x9p4t" (no look-alike characters).
func NewRecoveryCodes(n int) []string {
	const set = "abcdefghjkmnpqrstuvwxyz23456789"
	out := make([]string, n)
	for i := range out {
		b := make([]byte, 10)
		for j := range b {
			x, err := rand.Int(rand.Reader, big.NewInt(int64(len(set)))) // uniform, no modulo bias
			if err != nil {
				panic("totp: no randomness available: " + err.Error())
			}
			b[j] = set[x.Int64()]
		}
		out[i] = string(b[:5]) + "-" + string(b[5:])
	}
	return out
}

// NormRecoveryCode lowercases a recovery code and drops spaces and dashes before hashing.
func NormRecoveryCode(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.NewReplacer("-", "", " ", "").Replace(s)
}
