package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Offline cache: the last successful fetch for (server, token, path) is kept on disk so
// `vaultly run` still starts an app when the server is unreachable.
//
//   - Encrypted with AES-256-GCM under a key derived (HKDF-SHA256) from the token itself.
//     A cache file copied off the machine (backups, synced folders) is useless without the
//     token, and holding the token already lets you fetch, so the cache grants nothing new.
//   - The file name is a hash of (server, token, path) and reveals none of them.
//   - Used only for network failures and 5xx, never for 401/403/404: a revoked or expired token
//     must stop working at once, so those responses also delete the entry.
//   - Entries older than --max-age (default 24h) are ignored.

type cacheEntry struct {
	FetchedAt time.Time         `json:"fetched_at"`
	Secrets   map[string]string `json:"secrets"`
}

type cache struct {
	dir   string
	name  string
	key   []byte
	label []byte // AAD: binds the ciphertext to this file name
}

func cacheDir() (string, error) {
	if d := os.Getenv("VAULTLY_CACHE_DIR"); d != "" {
		return d, nil
	}
	d, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "vaultly"), nil
}

func newCache(dir, url, token, path string) (*cache, error) {
	id := sha256.Sum256([]byte("vaultly-cache-id\x00" + url + "\x00" + token + "\x00" + path))
	key, err := hkdf.Key(sha256.New, []byte(token), []byte("vaultly-offline-cache-v1"), "secrets", 32)
	if err != nil {
		return nil, err
	}
	name := hex.EncodeToString(id[:16]) + ".bin"
	return &cache{dir: dir, name: name, key: key, label: []byte(name)}, nil
}

func (c *cache) file() string { return filepath.Join(c.dir, c.name) }

func (c *cache) gcm() (cipher.AEAD, error) {
	b, err := aes.NewCipher(c.key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(b)
}

func (c *cache) save(secrets map[string]string, now time.Time) error {
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return err
	}
	plain, _ := json.Marshal(cacheEntry{FetchedAt: now.UTC(), Secrets: secrets})
	g, err := c.gcm()
	if err != nil {
		return err
	}
	nonce := make([]byte, g.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	return writeFileAtomic(c.file(), g.Seal(nonce, nonce, plain, c.label), 0o600)
}

var errCacheMiss = errors.New("no usable cached secrets")

// load returns the entry if it exists, decrypts, and is younger than maxAge.
func (c *cache) load(maxAge time.Duration, now time.Time) (*cacheEntry, error) {
	b, err := os.ReadFile(c.file())
	if err != nil {
		return nil, errCacheMiss
	}
	g, err := c.gcm()
	if err != nil {
		return nil, err
	}
	if len(b) < g.NonceSize() {
		return nil, errCacheMiss
	}
	plain, err := g.Open(nil, b[:g.NonceSize()], b[g.NonceSize():], c.label)
	if err != nil {
		return nil, errCacheMiss // wrong token or tampered file
	}
	var e cacheEntry
	if err := json.Unmarshal(plain, &e); err != nil {
		return nil, errCacheMiss
	}
	if maxAge <= 0 || now.Sub(e.FetchedAt) > maxAge {
		return nil, errCacheMiss
	}
	return &e, nil
}

func (c *cache) drop() { _ = os.Remove(c.file()) }
