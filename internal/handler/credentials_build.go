package handler

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/proxy"
	"github.com/usehivy/hivy/internal/registry"
)

func (h *CredentialHandler) buildCredential(ctx context.Context, orgID *uuid.UUID, req createCredentialRequest) (model.Credential, error) {
	if req.APIKey == "" || req.ProviderID == "" {
		return model.Credential{}, fmt.Errorf("provider_id and api_key are required")
	}
	provider, ok := registry.Global().GetProvider(req.ProviderID)
	if !ok {
		return model.Credential{}, fmt.Errorf("unknown provider_id %q", req.ProviderID)
	}
	if req.BaseURL == "" {
		if provider.API == "" {
			return model.Credential{}, fmt.Errorf("provider %q does not have a base URL configured; please provide base_url", req.ProviderID)
		}
		req.BaseURL = provider.API
	}
	if req.AuthScheme == "" {
		req.AuthScheme = defaultCredentialAuthScheme(req.ProviderID)
	}
	if !validCredentialAuthScheme(req.AuthScheme) {
		return model.Credential{}, fmt.Errorf("invalid auth_scheme")
	}
	if err := proxy.ValidateBaseURL(req.BaseURL); err != nil {
		return model.Credential{}, fmt.Errorf("invalid base_url: %v", err)
	}
	if req.RefillInterval != nil {
		if _, err := time.ParseDuration(*req.RefillInterval); err != nil {
			return model.Credential{}, fmt.Errorf("invalid refill_interval: must be a valid Go duration (e.g. 1h, 24h)")
		}
	}
	encryptedKey, wrappedDEK, err := h.encryptAPIKey(ctx, req.APIKey)
	if err != nil {
		return model.Credential{}, err
	}
	return model.Credential{
		ID:             uuid.New(),
		OrgID:          orgID,
		Label:          req.Label,
		BaseURL:        req.BaseURL,
		AuthScheme:     req.AuthScheme,
		ProviderID:     req.ProviderID,
		EncryptedKey:   encryptedKey,
		WrappedDEK:     wrappedDEK,
		Remaining:      req.Remaining,
		RefillAmount:   req.RefillAmount,
		RefillInterval: req.RefillInterval,
		Meta:           req.Meta,
	}, nil
}

func (h *CredentialHandler) encryptAPIKey(ctx context.Context, apiKey string) ([]byte, []byte, error) {
	dek, err := crypto.GenerateDEK()
	if err != nil {
		return nil, nil, fmt.Errorf("encryption failed")
	}
	encryptedKey, err := crypto.EncryptCredential([]byte(apiKey), dek)
	if err != nil {
		return nil, nil, fmt.Errorf("encryption failed")
	}
	wrappedDEK, err := h.kms.Wrap(ctx, dek)
	if err != nil {
		return nil, nil, fmt.Errorf("key wrapping failed")
	}
	for i := range dek {
		dek[i] = 0
	}
	return encryptedKey, wrappedDEK, nil
}
