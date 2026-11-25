package dynamic

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type Config struct {
	AdminURL string   `json:"admin_url"`
	Grants   []string `json:"grants"`
}

type Provider interface {
	Create(ctx context.Context, cfg Config, username, password string, expires time.Time) error
	Revoke(ctx context.Context, cfg Config, username string) error
	ConnectionURL(cfg Config, username, password string) (string, error)
	Validate(ctx context.Context, cfg Config) error
}

type Postgres struct{}

var usernameRe = regexp.MustCompile(`^v_[a-z0-9]{16}$`)

func quote(name string) (string, error) {
	if !usernameRe.MatchString(name) {
		return "", fmt.Errorf("invalid generated role name")
	}
	return `"` + name + `"`, nil
}

func (Postgres) Validate(ctx context.Context, cfg Config) error {
	u, err := url.Parse(cfg.AdminURL)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Host == "" {
		return fmt.Errorf("admin_url must be a postgres:// connection string")
	}
	conn, err := pgx.Connect(ctx, cfg.AdminURL)
	if err != nil {
		return fmt.Errorf("could not connect with the admin credentials: %w", err)
	}
	defer conn.Close(ctx)
	var canCreate bool
	if err := conn.QueryRow(ctx, `SELECT rolcreaterole OR rolsuper FROM pg_roles WHERE rolname = current_user`).Scan(&canCreate); err != nil || !canCreate {
		return fmt.Errorf("the admin user needs the CREATEROLE privilege")
	}
	return nil
}

func (Postgres) Create(ctx context.Context, cfg Config, username, password string, expires time.Time) error {
	role, err := quote(username)
	if err != nil {
		return err
	}
	if strings.ContainsAny(password, `'\`) {
		return fmt.Errorf("invalid generated password")
	}
	conn, err := pgx.Connect(ctx, cfg.AdminURL)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	stmt := fmt.Sprintf(`CREATE ROLE %s WITH LOGIN PASSWORD '%s' VALID UNTIL '%s'`, role, password, expires.UTC().Format("2006-01-02 15:04:05+00"))
	if _, err := conn.Exec(ctx, stmt); err != nil {
		return err
	}
	for _, g := range cfg.Grants {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		if _, err := conn.Exec(ctx, strings.ReplaceAll(g, "{{name}}", role)); err != nil {
			revoke(ctx, conn, username)
			return fmt.Errorf("grant %q failed: %w", g, err)
		}
	}
	return nil
}

func (Postgres) Revoke(ctx context.Context, cfg Config, username string) error {
	conn, err := pgx.Connect(ctx, cfg.AdminURL)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	return revoke(ctx, conn, username)
}

func revoke(ctx context.Context, conn *pgx.Conn, username string) error {
	role, err := quote(username)
	if err != nil {
		return err
	}
	conn.Exec(ctx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE usename = $1`, username)
	conn.Exec(ctx, fmt.Sprintf(`DROP OWNED BY %s`, role))
	_, err = conn.Exec(ctx, fmt.Sprintf(`DROP ROLE IF EXISTS %s`, role))
	return err
}

func (Postgres) ConnectionURL(cfg Config, username, password string) (string, error) {
	u, err := url.Parse(cfg.AdminURL)
	if err != nil {
		return "", err
	}
	u.User = url.UserPassword(username, password)
	return u.String(), nil
}

var _ Provider = Postgres{}
