package handler

import (
	"time"

	"github.com/usehivy/hivy/internal/model"
)

func toCredentialResponse(c model.Credential) credentialResponse {
	resp := credentialResponse{
		ID:             c.ID.String(),
		Label:          c.Label,
		BaseURL:        c.BaseURL,
		AuthScheme:     c.AuthScheme,
		ProviderID:     c.ProviderID,
		Remaining:      c.Remaining,
		RefillAmount:   c.RefillAmount,
		RefillInterval: c.RefillInterval,
		Meta:           c.Meta,
		CreatedAt:      c.CreatedAt.Format(time.RFC3339),
	}
	if c.RevokedAt != nil {
		s := c.RevokedAt.Format(time.RFC3339)
		resp.RevokedAt = &s
	}
	return resp
}
