// Package tokenutil creates the random, prefixed credentials used across Vaultly.
// Only the SHA-256 hash is ever stored; the raw value is shown once.
package tokenutil

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"math/big"
)

const (
	PrefixService  = "vst_"
	PrefixPersonal = "vpt_"
	PrefixInvite   = "vin_"
)

func New(prefix string) (raw string, hash []byte) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("tokenutil: no randomness available: " + err.Error())
	}
	raw = prefix + base64.RawURLEncoding.EncodeToString(b)
	return raw, Hash(raw)
}

func Hash(raw string) []byte {
	h := sha256.Sum256([]byte(raw))
	return h[:]
}

const (
	alphabet   = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"
	lowerAlnum = "abcdefghijklmnopqrstuvwxyz0123456789"
)

// RandomString returns n random characters without look-alikes (0/O, 1/l/I).
func RandomString(n int) (string, error) { return pick(alphabet, n) }

// RandomLower returns n random lowercase letters and digits.
func RandomLower(n int) (string, error) { return pick(lowerAlnum, n) }

func pick(set string, n int) (string, error) {
	b := make([]byte, n)
	for i := range b {
		x, err := rand.Int(rand.Reader, big.NewInt(int64(len(set))))
		if err != nil {
			return "", err
		}
		b[i] = set[x.Int64()]
	}
	return string(b), nil
}
