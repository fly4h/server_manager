package credential

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Manager encrypts/decrypts secrets with AES-256-GCM.
type Manager struct {
	aead cipher.AEAD
}

// NewManager derives the key from a secret string. If the secret is empty,
// it loads (or generates) a random key file under dataDir so a fresh
// deployment works without manual setup while staying machine-local.
func NewManager(secret, dataDir string) (*Manager, error) {
	var key []byte
	switch {
	case secret != "":
		key = deriveKey(secret)
	default:
		keyFile := filepath.Join(dataDir, ".encryption_key")
		data, err := os.ReadFile(keyFile)
		if err == nil && len(data) == 64 {
			if k, err := hexDecode(string(data)); err == nil && len(k) == 32 {
				key = k
			}
		}
		if key == nil {
			k := make([]byte, 32)
			if _, err := rand.Read(k); err != nil {
				return nil, err
			}
			if err := os.WriteFile(keyFile, []byte(hexEncode(k)), 0o600); err != nil {
				return nil, fmt.Errorf("persist encryption key: %w", err)
			}
			key = k
		}
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Manager{aead: aead}, nil
}

func deriveKey(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}

// Encrypt seals plaintext; output is base64(nonce || ciphertext).
func (m *Manager) Encrypt(plaintext []byte) (string, error) {
	nonce := make([]byte, m.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := m.aead.Seal(nil, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(append(nonce, sealed...)), nil
}

// Decrypt opens a value produced by Encrypt.
func (m *Manager) Decrypt(encoded string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	if len(raw) < m.aead.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}
	nonce, ct := raw[:m.aead.NonceSize()], raw[m.aead.NonceSize():]
	return m.aead.Open(nil, nonce, ct, nil)
}

func hexEncode(b []byte) string {
	const hexDigits = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = hexDigits[v>>4]
		out[i*2+1] = hexDigits[v&0x0f]
	}
	return string(out)
}

func hexDecode(s string) ([]byte, error) {
	if len(s)%2 != 0 {
		return nil, errors.New("bad hex length")
	}
	out := make([]byte, len(s)/2)
	for i := 0; i < len(out); i++ {
		hi := hexVal(s[i*2])
		lo := hexVal(s[i*2+1])
		if hi < 0 || lo < 0 {
			return nil, errors.New("bad hex digit")
		}
		out[i] = byte(hi<<4 | lo)
	}
	return out, nil
}

func hexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}
