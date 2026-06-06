package gateway

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
)

const externalSecretPrefix = "hvgw_"

func GenerateExternalGatewaySecret() (plaintext, hash, prefix string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", "", fmt.Errorf("generate external gateway secret: %w", err)
	}
	plaintext = externalSecretPrefix + hex.EncodeToString(b)
	hash = HashExternalGatewaySecret(plaintext)
	prefix = plaintext[:14]
	return plaintext, hash, prefix, nil
}

func HashExternalGatewaySecret(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

func VerifyExternalGatewaySecret(plaintext, expectedHash string) bool {
	plaintext = strings.TrimSpace(plaintext)
	expectedHash = strings.TrimSpace(expectedHash)
	if !strings.HasPrefix(plaintext, externalSecretPrefix) || expectedHash == "" {
		return false
	}
	got := HashExternalGatewaySecret(plaintext)
	return subtle.ConstantTimeCompare([]byte(got), []byte(expectedHash)) == 1
}
