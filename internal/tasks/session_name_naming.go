package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/trigger/hivy"
)

const (
	sessionNameProviderID = "openrouter"
	sessionNameModelID    = "gpt-4o-mini"
	sessionNameMaxLen     = 80
)

const sessionNameResponseSchema = `{
	"type": "object",
	"additionalProperties": false,
	"properties": {
		"name": {"type": "string"}
	},
	"required": ["name"]
}`

var errSessionNameModelUnavailable = errors.New("session name model unavailable")

type sessionNameCredential struct {
	credential *model.Credential
	apiKey     string
	modelID    string
}

type sessionNameCredentialLoader func(
	ctx context.Context,
	db *gorm.DB,
	cacheManager *cache.Manager,
	reg *registry.Registry,
) (*sessionNameCredential, error)

type sessionNameClientFactory func(credential *model.Credential, apiKey string) hivy.CompletionClient

func (handler *SessionNameHandler) registry() *registry.Registry {
	if handler != nil && handler.reg != nil {
		return handler.reg
	}
	return registry.Global()
}

func loadSessionNameCredential(
	ctx context.Context,
	db *gorm.DB,
	cacheManager *cache.Manager,
	reg *registry.Registry,
) (*sessionNameCredential, error) {
	if db == nil || cacheManager == nil {
		return nil, fmt.Errorf("missing naming dependencies")
	}
	if reg == nil {
		reg = registry.Global()
	}

	route, ok := reg.ResolveModel(sessionNameProviderID, sessionNameModelID)
	if !ok {
		return nil, fmt.Errorf(
			"%w: provider=%q model=%q",
			errSessionNameModelUnavailable,
			sessionNameProviderID,
			sessionNameModelID,
		)
	}

	credential, err := pickSessionNameCredential(ctx, db, reg)
	if err != nil {
		return nil, err
	}

	decrypted, err := cacheManager.GetDecryptedCredentialByID(ctx, credential.ID.String())
	if err != nil {
		return nil, fmt.Errorf("decrypt system credential: %w", err)
	}

	return &sessionNameCredential{
		credential: credential,
		apiKey:     string(decrypted.APIKey),
		modelID:    route.UpstreamID,
	}, nil
}

func pickSessionNameCredential(
	ctx context.Context,
	db *gorm.DB,
	reg *registry.Registry,
) (*model.Credential, error) {
	if reg == nil {
		reg = registry.Global()
	}
	if err := reg.ValidateCanonicalModel(sessionNameModelID); err != nil {
		return nil, err
	}

	var creds []model.Credential
	if err := db.WithContext(ctx).
		Where("org_id IS NULL AND revoked_at IS NULL AND provider_id = ?", sessionNameProviderID).
		Order("created_at ASC").
		Find(&creds).Error; err != nil {
		return nil, fmt.Errorf("list active session naming credentials: %w", err)
	}

	for index := range creds {
		if _, ok := reg.ResolveModel(creds[index].ProviderID, sessionNameModelID); ok {
			return &creds[index], nil
		}
	}

	return nil, fmt.Errorf(
		"%w: provider=%q model=%q",
		credentials.ErrNoSystemCredential,
		sessionNameProviderID,
		sessionNameModelID,
	)
}

func generateSessionName(
	ctx context.Context,
	client hivy.CompletionClient,
	modelID string,
	firstMessage string,
) (string, error) {
	systemPrompt := strings.Join([]string{
		"You generate concise Hivy session names.",
		`Return strict JSON only in the shape {"name":"..."}.`,
		"Generate a short, specific session name from the user's first message.",
		"Use lowercase words separated by single dashes.",
		"Prefer 3-8 words.",
		"Preserve meaningful identifiers when present, such as company names, repo names, issue IDs, ticket IDs, tools, or provider names.",
		`Do not include any keys other than "name".`,
	}, " ")
	userPrompt := "First user message:\n\n" + firstMessage

	req := hivy.CompletionRequest{
		Model: modelID,
		Messages: []hivy.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		MaxTokens: 48,
		ResponseFormat: &hivy.ResponseFormat{
			Type: hivy.ResponseFormatJSONSchema,
			JSONSchema: &hivy.ResponseJSONSchema{
				Name:   "session_name",
				Schema: json.RawMessage(sessionNameResponseSchema),
				Strict: true,
			},
		},
	}

	resp, err := client.ChatCompletion(ctx, req)
	if err != nil {
		return "", err
	}

	var parsed struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(resp.Message.Content)), &parsed); err != nil {
		return "", fmt.Errorf("decode session name json: %w", err)
	}
	return parsed.Name, nil
}

func normalizeGeneratedSessionName(raw string) string {
	name := model.GenerateSlug(raw)
	if name == "" {
		return ""
	}
	if len(name) > sessionNameMaxLen {
		name = strings.Trim(name[:sessionNameMaxLen], "-")
	}
	return name
}
