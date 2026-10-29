package crypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/crypto/argon2"
)

var (
	ErrNotEncrypted = errors.New("not encrypted")
	ErrEncrypted    = errors.New("file is encrypted, a key is required")
	ErrBadKey       = errors.New("wrong key")
)

const (
	magic   = "TAILFENC1"
	saltLen = 32
)

var keyParams = struct {
	time    uint32
	memory  uint32
	threads uint8
}{time: 3, memory: 64 * 1024, threads: 4}

func derive(password string, salt []byte) (cipher.Block, error) {
	return aes.NewCipher(argon2.IDKey([]byte(password), salt, keyParams.time, keyParams.memory, keyParams.threads, 32))
}

func random(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

func Seal(plaintext []byte, password string) ([]byte, error) {
	if password == "" {
		return nil, errors.New("empty key")
	}
	salt := random(saltLen)
	block, err := derive(password, salt)
	if err != nil {
		return nil, err
	}
	g, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := random(g.NonceSize())
	ct := g.Seal(nil, nonce, plaintext, nil)
	out := make([]byte, 0, len(magic)+saltLen+len(nonce)+len(ct))
	out = append(out, magic...)
	out = append(out, salt...)
	out = append(out, nonce...)
	return append(out, ct...), nil
}

func Open(sealed []byte, password string) ([]byte, error) {
	if password == "" {
		return nil, ErrEncrypted
	}
	if !IsEncrypted(sealed) {
		return nil, ErrNotEncrypted
	}
	off := len(magic)
	salt := sealed[off : off+saltLen]
	off += saltLen
	block, err := derive(password, salt)
	if err != nil {
		return nil, err
	}
	g, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(sealed) < off+g.NonceSize()+g.Overhead() {
		return nil, ErrBadKey
	}
	nonce := sealed[off : off+g.NonceSize()]
	ct := sealed[off+g.NonceSize():]
	plain, err := g.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, ErrBadKey
	}
	return plain, nil
}

func IsEncrypted(data []byte) bool {
	return len(data) >= len(magic) && string(data[:len(magic)]) == magic
}

func ReadFile(path, key string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if !IsEncrypted(data) {
		return data, nil
	}
	return Open(data, key)
}

func WriteFile(path string, data []byte, key string) error {
	out := data
	if key != "" {
		var err error
		out, err = Seal(data, key)
		if err != nil {
			return err
		}
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() {
		_ = os.Remove(name)
	}()
	if _, err := tmp.Write(out); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(path)
		return os.Rename(name, path)
	}
	return nil
}
