package dynamic

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func adminURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://vaultly:vaultly@localhost:5432/vaultly?sslmode=disable"
	}
	conn, err := pgx.Connect(context.Background(), url)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	conn.Close(context.Background())
	return url
}

func canLogin(url string) error {
	conn, err := pgx.Connect(context.Background(), url)
	if err != nil {
		return err
	}
	defer conn.Close(context.Background())
	var one int
	return conn.QueryRow(context.Background(), `SELECT 1`).Scan(&one)
}

func TestLeaseCreatesWorkingRoleThatRevokeRemoves(t *testing.T) {
	cfg := Config{AdminURL: adminURL(t), Grants: []string{`GRANT pg_read_all_data TO {{name}}`}}
	p := Postgres{}
	ctx := context.Background()
	if err := p.Validate(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	user, pass := "v_testlease0000001", "Abc123Def456Ghi789"
	if err := p.Create(ctx, cfg, user, pass, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	connURL, err := p.ConnectionURL(cfg, user, pass)
	if err != nil {
		t.Fatal(err)
	}
	if err := canLogin(connURL); err != nil {
		t.Fatalf("leased credentials should log in: %v", err)
	}
	if err := p.Revoke(ctx, cfg, user); err != nil {
		t.Fatal(err)
	}
	if err := canLogin(connURL); err == nil {
		t.Fatal("revoked credentials must no longer work")
	}
	if err := p.Revoke(ctx, cfg, user); err != nil {
		t.Fatalf("revoke should be idempotent: %v", err)
	}
}

func TestFailedGrantRollsBackTheRole(t *testing.T) {
	cfg := Config{AdminURL: adminURL(t), Grants: []string{`GRANT no_such_role TO {{name}}`}}
	p := Postgres{}
	user := "v_testbadgrant00001"[:18]
	if err := p.Create(context.Background(), cfg, user, "Abc123Def456", time.Now().Add(time.Hour)); err == nil {
		t.Fatal("expected grant failure")
	}
	url, _ := p.ConnectionURL(cfg, user, "Abc123Def456")
	if err := canLogin(url); err == nil {
		t.Fatal("role must not survive a failed grant")
	}
}

func TestRejectsUnsafeNames(t *testing.T) {
	p := Postgres{}
	for _, bad := range []string{`v_x"; DROP TABLE users; --`, "postgres", "v_UPPERCASE0000000", ""} {
		if err := p.Create(context.Background(), Config{AdminURL: "postgres://x"}, bad, "pw", time.Now()); err == nil {
			t.Errorf("name %q should be rejected", bad)
		}
	}
	if err := p.Validate(context.Background(), Config{AdminURL: "mysql://x"}); err == nil || !strings.Contains(err.Error(), "postgres://") {
		t.Errorf("non-postgres URL should be rejected, got %v", err)
	}
}
