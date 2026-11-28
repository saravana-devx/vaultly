package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

const (
	AccessTTL  = 15 * time.Minute
	RefreshTTL = 7 * 24 * time.Hour
)

type Manager struct {
	secret []byte
	rdb    *redis.Client
}

func NewManager(secret string, rdb *redis.Client) *Manager {
	return &Manager{secret: []byte(secret), rdb: rdb}
}

func HashPassword(p string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(p), 12)
	return string(b), err
}

func CheckPassword(hash, p string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(p)) == nil
}

type Claims struct {
	Email string `json:"email"`
	jwt.RegisteredClaims
}

func (m *Manager) NewAccessToken(userID uuid.UUID, email string) (string, error) {
	now := time.Now()
	c := Claims{Email: email, RegisteredClaims: jwt.RegisteredClaims{
		Subject: userID.String(), IssuedAt: jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(AccessTTL)), Issuer: "vaultly",
	}}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(m.secret)
}

func (m *Manager) ParseAccessToken(s string) (*Claims, error) {
	c := &Claims{}
	t, err := jwt.ParseWithClaims(s, c, func(*jwt.Token) (any, error) { return m.secret, nil },
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer("vaultly"))
	if err != nil || !t.Valid {
		return nil, errors.New("invalid token")
	}
	return c, nil
}

// MFA challenges bridge the two login steps: the password step returns one instead of a session,
// and the code step redeems it. Stored hashed, 5-minute TTL, dead after MaxMFAAttempts wrong codes.
const (
	MFAChallengeTTL = 5 * time.Minute
	MaxMFAAttempts  = 5
)

func (m *Manager) NewMFAChallenge(ctx context.Context, userID uuid.UUID) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	tok := base64.RawURLEncoding.EncodeToString(b)
	k := mfaKey(tok)
	pipe := m.rdb.TxPipeline()
	pipe.HSet(ctx, k, "uid", userID.String(), "fails", 0)
	pipe.Expire(ctx, k, MFAChallengeTTL)
	_, err := pipe.Exec(ctx)
	return tok, err
}

// PeekMFAChallenge returns the user a live challenge belongs to without consuming it.
func (m *Manager) PeekMFAChallenge(ctx context.Context, tok string) (uuid.UUID, error) {
	v, err := m.rdb.HGet(ctx, mfaKey(tok), "uid").Result()
	if err != nil {
		return uuid.Nil, errors.New("invalid challenge")
	}
	return uuid.Parse(v)
}

// FailMFAChallenge counts a wrong code and deletes the challenge once the limit is reached.
func (m *Manager) FailMFAChallenge(ctx context.Context, tok string) {
	k := mfaKey(tok)
	if n, err := m.rdb.HIncrBy(ctx, k, "fails", 1).Result(); err == nil && n >= MaxMFAAttempts {
		m.rdb.Del(ctx, k)
	}
}

// ConsumeMFAChallenge deletes the challenge; it reports false if it was already gone (single use).
func (m *Manager) ConsumeMFAChallenge(ctx context.Context, tok string) bool {
	n, err := m.rdb.Del(ctx, mfaKey(tok)).Result()
	return err == nil && n == 1
}

func mfaKey(tok string) string {
	h := sha256.Sum256([]byte(tok))
	return "mfa:" + hex.EncodeToString(h[:])
}

// Refresh tokens are opaque, stored hashed in Redis, and rotated on every use.
func (m *Manager) NewRefreshToken(ctx context.Context, userID uuid.UUID) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	tok := base64.RawURLEncoding.EncodeToString(b)
	return tok, m.rdb.Set(ctx, rtKey(tok), userID.String(), RefreshTTL).Err()
}

// ConsumeRefreshToken validates and deletes the token (single use).
func (m *Manager) ConsumeRefreshToken(ctx context.Context, tok string) (uuid.UUID, error) {
	v, err := m.rdb.GetDel(ctx, rtKey(tok)).Result()
	if err != nil {
		return uuid.Nil, errors.New("invalid refresh token")
	}
	return uuid.Parse(v)
}

func (m *Manager) RevokeRefreshToken(ctx context.Context, tok string) {
	m.rdb.Del(ctx, rtKey(tok))
}

func rtKey(tok string) string {
	h := sha256.Sum256([]byte(tok))
	return "rt:" + hex.EncodeToString(h[:])
}
