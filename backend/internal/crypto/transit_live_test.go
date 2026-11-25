package crypto

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// Runs only when pointed at a real OpenBao (BAO_ADDR + BAO_TOKEN_FILE), e.g. the compose stack.
func TestLiveOpenBaoTransit(t *testing.T) {
	addr, file := os.Getenv("BAO_ADDR"), os.Getenv("BAO_TOKEN_FILE")
	if addr == "" || file == "" {
		t.Skip("BAO_ADDR / BAO_TOKEN_FILE not set")
	}
	tr := NewTransit(addr, "transit", "vaultly", func() (string, error) {
		b, err := os.ReadFile(file)
		return strings.TrimSpace(string(b)), err
	}, nil)
	if err := tr.Check(ctx); err != nil {
		t.Fatal(err)
	}
	plain, wrapped, err := tr.NewDEK(ctx)
	if err != nil || !IsTransit(wrapped) {
		t.Fatalf("NewDEK: %v", err)
	}
	again, err := tr.Rewrap(ctx, wrapped)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := tr.UnwrapDEK(ctx, again); err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("round trip through a real OpenBao failed: %v", err)
	}
	bad := NewTransit(addr, "transit", "vaultly", func() (string, error) { return "not-a-token", nil }, nil)
	if _, _, err := bad.NewDEK(ctx); err == nil {
		t.Fatal("an invalid token must be rejected by OpenBao")
	}
	other := NewTransit(addr, "transit", "some-other-key", func() (string, error) {
		b, _ := os.ReadFile(file)
		return strings.TrimSpace(string(b)), nil
	}, nil)
	if _, _, err := other.NewDEK(ctx); err == nil {
		t.Fatal("the app token must be limited to the vaultly key (least privilege)")
	}
}
