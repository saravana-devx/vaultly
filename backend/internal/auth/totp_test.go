package auth

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

// RFC 6238 appendix B, SHA-1, truncated to 6 digits. Secret is ASCII "12345678901234567890".
const rfcSecret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

func TestTOTPRFCVectors(t *testing.T) {
	cases := []struct {
		unix int64
		want string
	}{
		{59, "287082"},
		{1111111109, "081804"},
		{1111111111, "050471"},
		{1234567890, "005924"},
		{2000000000, "279037"},
		{20000000000, "353130"},
	}
	for _, c := range cases {
		got, err := TOTPCode(rfcSecret, TOTPStep(time.Unix(c.unix, 0)))
		if err != nil || got != c.want {
			t.Errorf("t=%d: got %s, %v; want %s", c.unix, got, err, c.want)
		}
	}
}

func TestVerifyTOTPWindow(t *testing.T) {
	now := time.Unix(1234567890, 0)
	step := TOTPStep(now)
	code := func(s int64) string { c, _ := TOTPCode(rfcSecret, s); return c }

	for _, d := range []int64{-1, 0, 1} {
		got, ok := VerifyTOTP(rfcSecret, code(step+d), now)
		if !ok || got != step+d {
			t.Errorf("drift %d: got step %d, ok=%v", d, got, ok)
		}
	}
	for _, d := range []int64{-2, 2} {
		if _, ok := VerifyTOTP(rfcSecret, code(step+d), now); ok {
			t.Errorf("drift %d should be outside the window", d)
		}
	}
	if _, ok := VerifyTOTP(rfcSecret, "005 924", now); !ok {
		t.Error("spaces inside the code should be ignored")
	}
	for _, bad := range []string{"", "12345", "1234567", "abcdef"} {
		if _, ok := VerifyTOTP(rfcSecret, bad, now); ok {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestNewTOTPSecret(t *testing.T) {
	s := NewTOTPSecret()
	if len(s) != 32 { // 20 bytes -> 32 base32 characters
		t.Fatalf("unexpected secret length %d", len(s))
	}
	if s == NewTOTPSecret() {
		t.Fatal("secrets must be random")
	}
	if _, err := TOTPCode(s, 1); err != nil {
		t.Fatal(err)
	}
}

func TestTOTPURL(t *testing.T) {
	u, err := url.Parse(TOTPURL("ABC", "a@b.c"))
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "otpauth" || u.Host != "totp" || u.Path != "/Vaultly:a@b.c" {
		t.Fatalf("bad url %s", u)
	}
	q := u.Query()
	if q.Get("secret") != "ABC" || q.Get("issuer") != "Vaultly" || q.Get("digits") != "6" || q.Get("period") != "30" {
		t.Fatalf("bad query %v", q)
	}
	qr, err := TOTPQR(u.String())
	if err != nil || !strings.HasPrefix(qr, "data:image/png;base64,") {
		t.Fatalf("bad QR: %.40s %v", qr, err)
	}
}

func TestRecoveryCodes(t *testing.T) {
	codes := NewRecoveryCodes(10)
	seen := map[string]bool{}
	for _, c := range codes {
		if len(c) != 11 || c[5] != '-' {
			t.Fatalf("bad format %q", c)
		}
		if strings.ContainsAny(c, "01ilo") {
			t.Fatalf("look-alike characters in %q", c)
		}
		if seen[c] {
			t.Fatal("duplicate code")
		}
		seen[c] = true
	}
	if NormRecoveryCode(" K7M2Q-X9P4T ") != "k7m2qx9p4t" || NormRecoveryCode("k7m2q x9p4t") != "k7m2qx9p4t" {
		t.Fatal("normalisation should ignore case, dashes and spaces")
	}
}
