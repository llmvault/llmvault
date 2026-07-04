package handler

import (
	"crypto/subtle"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/model"
)

// resolveSandboxBySecret returns the agent's sandbox whose runtime secret matches
// the bearer token, in constant time. Sandbox runtime secrets are the credential
// sandboxes present when calling control-plane endpoints (git-credentials,
// gh-wrapper report). Because a sandbox belongs to exactly one active session,
// the matched sandbox pins the caller's session.
func resolveSandboxBySecret(db *gorm.DB, encKey *crypto.SymmetricKey, agentID uuid.UUID, bearer string) (model.Sandbox, bool) {
	if db == nil || encKey == nil || agentID == uuid.Nil || bearer == "" {
		return model.Sandbox{}, false
	}
	var sandboxes []model.Sandbox
	if err := db.Where("agent_id = ?", agentID).Find(&sandboxes).Error; err != nil {
		return model.Sandbox{}, false
	}
	for _, sb := range sandboxes {
		decrypted, err := encKey.DecryptString(sb.EncryptedRuntimeSecret)
		if err != nil {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(bearer), []byte(decrypted)) == 1 {
			return sb, true
		}
	}
	return model.Sandbox{}, false
}
