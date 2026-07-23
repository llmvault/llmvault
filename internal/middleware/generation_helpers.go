package middleware

import (
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func lookupProviderID(db *gorm.DB, credentialID string) string {
	var providerID string
	db.Model(&model.Credential{}).Select("provider_id").Where("id = ?", credentialID).Scan(&providerID)
	return providerID
}

func truncateValidUTF8(s string, maxLen int) string {
	s = strings.ToValidUTF8(s, "?")
	s = strings.ReplaceAll(s, "\x00", "?")
	if len(s) <= maxLen {
		return s
	}
	return strings.ToValidUTF8(s[:maxLen], "?")
}
