package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port         string
	DatabaseURL  string
	RedisURL     string
	KafkaBrokers []string
	AuditTopic   string
	MasterKey    string // base64, 32 bytes: the local KEK (required for KEY_PROVIDER=local, legacy fallback for openbao)
	JWTSecret    string
	CORSOrigins  []string

	AppURL   string
	SMTPHost string
	SMTPPort string
	SMTPUser string
	SMTPPass string
	SMTPFrom string

	WebhookAllowPrivate bool

	KeyProvider  string // local | openbao
	BaoAddr      string
	BaoToken     string
	BaoTokenFile string
	BaoMount     string
	BaoKey       string
	KeyCacheTTL  time.Duration
}

func Load() (*Config, error) {
	c := &Config{
		Port:        get("PORT", "8080"),
		DatabaseURL: get("DATABASE_URL", "postgres://vaultly:vaultly@localhost:5432/vaultly?sslmode=disable"),
		RedisURL:    get("REDIS_URL", "redis://localhost:6379/0"),
		AuditTopic:  get("KAFKA_AUDIT_TOPIC", "vaultly.audit"),
		MasterKey:   os.Getenv("MASTER_KEY"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		AppURL:      strings.TrimRight(get("APP_URL", "http://localhost:3000"), "/"),
		SMTPHost:    os.Getenv("SMTP_HOST"),
		SMTPPort:    get("SMTP_PORT", "587"),
		SMTPUser:    os.Getenv("SMTP_USER"),
		SMTPPass:    os.Getenv("SMTP_PASS"),
		SMTPFrom:    get("SMTP_FROM", "no-reply@vaultly.local"),

		WebhookAllowPrivate: os.Getenv("WEBHOOK_ALLOW_PRIVATE") == "true",

		KeyProvider:  strings.ToLower(get("KEY_PROVIDER", "local")),
		BaoAddr:      os.Getenv("BAO_ADDR"),
		BaoToken:     os.Getenv("BAO_TOKEN"),
		BaoTokenFile: os.Getenv("BAO_TOKEN_FILE"),
		BaoMount:     get("BAO_TRANSIT_MOUNT", "transit"),
		BaoKey:       get("BAO_KEY_NAME", "vaultly"),
	}
	if b := os.Getenv("KAFKA_BROKERS"); b != "" {
		c.KafkaBrokers = strings.Split(b, ",")
	}
	if o := get("CORS_ORIGINS", "http://localhost:5173,http://localhost:3000"); o != "" {
		c.CORSOrigins = strings.Split(o, ",")
	}
	ttl, err := strconv.Atoi(get("KEY_CACHE_TTL_SECONDS", "60"))
	if err != nil || ttl < 0 {
		return nil, fmt.Errorf("KEY_CACHE_TTL_SECONDS must be a non-negative integer")
	}
	c.KeyCacheTTL = time.Duration(ttl) * time.Second
	switch c.KeyProvider {
	case "local":
		if c.MasterKey == "" {
			return nil, fmt.Errorf("MASTER_KEY is required (generate with: openssl rand -base64 32)")
		}
	case "openbao":
		if c.BaoAddr == "" || (c.BaoToken == "" && c.BaoTokenFile == "") {
			return nil, fmt.Errorf("KEY_PROVIDER=openbao needs BAO_ADDR and BAO_TOKEN (or BAO_TOKEN_FILE)")
		}
	default:
		return nil, fmt.Errorf("KEY_PROVIDER must be local or openbao")
	}
	if len(c.JWTSecret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET is required and must be at least 32 characters")
	}
	return c, nil
}

func get(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
