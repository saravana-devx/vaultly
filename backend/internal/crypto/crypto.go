// Package crypto implements envelope encryption.
//
//	root key (KEK)  --wraps-->  per-project DEK  --encrypts-->  secret values
//
// The KEK never touches the database. It lives either in MASTER_KEY (Local) or inside
// OpenBao/Vault (Transit); both satisfy KeyProvider.
package crypto

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

// KeyProvider is the root of trust: it creates and unwraps per-project data keys.
// Implementations: Local (MASTER_KEY from the environment) and Transit (OpenBao/Vault).
type KeyProvider interface {
	Name() string
	NewDEK(ctx context.Context) (plain, wrapped []byte, err error)
	UnwrapDEK(ctx context.Context, wrapped []byte) ([]byte, error)
	// Rewrap protects an existing wrapped key under the provider's current key.
	Rewrap(ctx context.Context, wrapped []byte) ([]byte, error)
}

// Local wraps data keys with a 32-byte master key held in memory.
type Local struct{ kek []byte }

func NewLocal(masterKeyB64 string) (*Local, error) {
	k, err := base64.StdEncoding.DecodeString(masterKeyB64)
	if err != nil {
		return nil, fmt.Errorf("MASTER_KEY must be base64: %w", err)
	}
	if len(k) != 32 {
		return nil, fmt.Errorf("MASTER_KEY must decode to 32 bytes, got %d", len(k))
	}
	return &Local{kek: k}, nil
}

func (l *Local) Name() string { return "local" }

func (l *Local) NewDEK(context.Context) (plain, wrapped []byte, err error) {
	plain = make([]byte, 32)
	if _, err = rand.Read(plain); err != nil {
		return nil, nil, err
	}
	wrapped, err = l.Wrap(plain)
	return plain, wrapped, err
}

func (l *Local) Wrap(plain []byte) ([]byte, error) { return Encrypt(l.kek, plain, []byte("dek")) }

func (l *Local) UnwrapDEK(_ context.Context, wrapped []byte) ([]byte, error) {
	return Decrypt(l.kek, wrapped, []byte("dek"))
}

func (l *Local) Rewrap(ctx context.Context, wrapped []byte) ([]byte, error) {
	plain, err := l.UnwrapDEK(ctx, wrapped)
	if err != nil {
		return nil, err
	}
	return l.Wrap(plain)
}

var _ KeyProvider = (*Local)(nil)

// Encrypt uses AES-256-GCM. Output is nonce || ciphertext||tag.
func Encrypt(key, plaintext, aad []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, aad), nil
}

func Decrypt(key, blob, aad []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	if len(blob) < gcm.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}
	return gcm.Open(nil, blob[:gcm.NonceSize()], blob[gcm.NonceSize():], aad)
}

func newGCM(key []byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, errors.New("key must be 32 bytes")
	}
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(b)
}
