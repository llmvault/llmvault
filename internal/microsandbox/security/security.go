package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/blake2b"
)

func RandomToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func HashToken(token string) []byte {
	sum := blake2b.Sum256([]byte(token))
	return sum[:]
}

func CheckToken(token string, hash []byte) bool {
	got := HashToken(token)
	return subtle.ConstantTimeCompare(got, hash) == 1
}

func GeneratePreviewPassword() string {
	words := []string{
		"amber", "atlas", "bloom", "cedar", "cinder", "cobalt", "comet", "coral",
		"dune", "ember", "field", "frost", "harbor", "hazel", "indigo", "juniper",
		"lagoon", "linen", "maple", "meadow", "nectar", "olive", "onyx", "orbit",
		"pearl", "plum", "prairie", "quartz", "river", "saffron", "spruce", "summit",
		"thistle", "topaz", "umber", "violet", "willow", "zephyr",
	}
	picks := make([]string, 3)
	for i := range picks {
		var b [1]byte
		_, _ = rand.Read(b[:])
		picks[i] = words[int(b[0])%len(words)]
	}
	return strings.Join(picks, "-")
}

func EncryptString(keyMaterial, plaintext string) ([]byte, error) {
	if strings.TrimSpace(keyMaterial) == "" {
		return nil, fmt.Errorf("encryption key is required")
	}
	key := sha256.Sum256([]byte(keyMaterial))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	return append(nonce, ciphertext...), nil
}

func DecryptString(keyMaterial string, encrypted []byte) (string, error) {
	if strings.TrimSpace(keyMaterial) == "" {
		return "", fmt.Errorf("encryption key is required")
	}
	key := sha256.Sum256([]byte(keyMaterial))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(encrypted) < gcm.NonceSize() {
		return "", fmt.Errorf("ciphertext is too short")
	}
	nonce, ciphertext := encrypted[:gcm.NonceSize()], encrypted[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func ConstantTimeStringEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func ShortID(_ string) (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	token := make([]byte, 8)
	random := make([]byte, len(token))
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	for i, b := range random {
		token[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(token), nil
}
