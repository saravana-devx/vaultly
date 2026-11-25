// Package backup seals an organisation export with a passphrase.
// Format: "VLTYBK1" | salt(16) | nonce(12) | AES-256-GCM(gzip(json)).
package backup

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"

	"golang.org/x/crypto/scrypt"
)

var magic = []byte("VLTYBK1")

const (
	saltLen    = 16
	maxPlain   = 256 << 20
	scryptN    = 1 << 15
	minPassLen = 12
)

var ErrBadPassphrase = errors.New("wrong passphrase or corrupted backup")

func key(pass string, salt []byte) ([]byte, error) {
	return scrypt.Key([]byte(pass), salt, scryptN, 8, 1, 32)
}

func Seal(pass string, plain []byte) ([]byte, error) {
	if len(pass) < minPassLen {
		return nil, errors.New("passphrase must be at least 12 characters")
	}
	var zipped bytes.Buffer
	zw := gzip.NewWriter(&zipped)
	if _, err := zw.Write(plain); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	k, err := key(pass, salt)
	if err != nil {
		return nil, err
	}
	blk, _ := aes.NewCipher(k)
	gcm, _ := cipher.NewGCM(blk)
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := append(append(append([]byte{}, magic...), salt...), nonce...)
	return gcm.Seal(out, nonce, zipped.Bytes(), magic), nil
}

func Open(pass string, blob []byte) ([]byte, error) {
	if len(blob) < len(magic)+saltLen+12+16 || !bytes.Equal(blob[:len(magic)], magic) {
		return nil, errors.New("not a Vaultly backup file")
	}
	salt := blob[len(magic) : len(magic)+saltLen]
	k, err := key(pass, salt)
	if err != nil {
		return nil, err
	}
	blk, _ := aes.NewCipher(k)
	gcm, _ := cipher.NewGCM(blk)
	rest := blob[len(magic)+saltLen:]
	zipped, err := gcm.Open(nil, rest[:gcm.NonceSize()], rest[gcm.NonceSize():], magic)
	if err != nil {
		return nil, ErrBadPassphrase
	}
	zr, err := gzip.NewReader(bytes.NewReader(zipped))
	if err != nil {
		return nil, err
	}
	plain, err := io.ReadAll(io.LimitReader(zr, maxPlain+1))
	if err != nil {
		return nil, err
	}
	if len(plain) > maxPlain {
		return nil, errors.New("backup too large")
	}
	return plain, nil
}
