package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/providerheaders"
)

const (
	generationReconcileBatchSize = 200
	generationReconcileTimeout   = 4 * time.Minute
	// generationReconcileWindow bounds the sweep to recent rows so generations
	// whose usage OpenRouter genuinely never reports don't churn forever.
	generationReconcileWindow = 24 * time.Hour
)

// generationReconcileRetryDelays backs off between attempts against
// /api/v1/generation, which is not queryable for a short window after the proxied
// response returns (propagation delay).
var generationReconcileRetryDelays = []time.Duration{500 * time.Millisecond, 1500 * time.Millisecond, 3 * time.Second}

// credentialDecryptor resolves the plaintext upstream credential for a
// generation row (satisfied by *cache.Manager).
type credentialDecryptor interface {
	GetDecryptedCredentialByID(ctx context.Context, credentialID string) (*cache.DecryptedCredential, error)
}

// openRouterGenerationData is the subset of GET /api/v1/generation we consume.
type openRouterGenerationData struct {
	ID                     string `json:"id"`
	NativeTokensPrompt     int    `json:"native_tokens_prompt"`
	NativeTokensCompletion int    `json:"native_tokens_completion"`
}

type openRouterGenerationResponse struct {
	Data openRouterGenerationData `json:"data"`
}

// generationFetcher fetches usage for one OpenRouter generation id. found=false
// signals the record is not yet queryable (propagation delay), a retryable state.
type generationFetcher interface {
	fetchGeneration(ctx context.Context, baseURL string, apiKey []byte, id string) (data openRouterGenerationData, found bool, err error)
}

type reconcileRow struct {
	ID                     string `gorm:"column:id"`
	ProviderID             string `gorm:"column:provider_id"`
	Model                  string `gorm:"column:model"`
	CredentialID           string `gorm:"column:credential_id"`
	OpenRouterGenerationID string `gorm:"column:openrouter_generation_id"`
}

// GenerationReconcileHandler backfills token usage on zero-usage OpenRouter
// system generations from the authoritative generation endpoint so the billing
// batch can then charge them.
type GenerationReconcileHandler struct {
	db      *gorm.DB
	creds   credentialDecryptor
	fetcher generationFetcher
}

func NewGenerationReconcileHandler(db *gorm.DB, creds credentialDecryptor) *GenerationReconcileHandler {
	return &GenerationReconcileHandler{
		db:      db,
		creds:   creds,
		fetcher: &httpGenerationFetcher{client: &http.Client{Timeout: 20 * time.Second}},
	}
}

func (h *GenerationReconcileHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	ctx, cancel := context.WithTimeout(ctx, generationReconcileTimeout)
	defer cancel()

	rows, err := h.selectZeroUsageBatch(ctx, generationReconcileBatchSize)
	if err != nil {
		return fmt.Errorf("select zero-usage generations: %w", err)
	}

	var reconciled int
	for _, r := range rows {
		if ctx.Err() != nil {
			break
		}
		ok, err := h.reconcileRow(ctx, r)
		if err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "generation reconcile failed",
				"id", r.ID, "openrouter_generation_id", r.OpenRouterGenerationID, "error", err)
			continue
		}
		if ok {
			reconciled++
		}
	}

	if reconciled > 0 {
		logging.FromContext(ctx).InfoContext(ctx, "generation reconcile backfilled usage",
			"rows", reconciled, "scanned", len(rows))
	}
	return nil
}

func (h *GenerationReconcileHandler) selectZeroUsageBatch(ctx context.Context, limit int) ([]reconcileRow, error) {
	rows := []reconcileRow{}
	err := h.db.WithContext(ctx).Raw(`
		SELECT id, provider_id, model, credential_id::text AS credential_id,
		       openrouter_generation_id
		FROM generations
		WHERE is_system = TRUE
		  AND openrouter_generation_id IS NOT NULL
		  AND input_tokens = 0
		  AND output_tokens = 0
		  AND billed_at IS NULL
		  AND created_at >= ?
		ORDER BY created_at
		LIMIT ?
	`, time.Now().UTC().Add(-generationReconcileWindow), limit).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (h *GenerationReconcileHandler) reconcileRow(ctx context.Context, r reconcileRow) (bool, error) {
	cred, err := h.creds.GetDecryptedCredentialByID(ctx, r.CredentialID)
	if err != nil {
		return false, fmt.Errorf("decrypt credential: %w", err)
	}
	defer func() {
		for i := range cred.APIKey {
			cred.APIKey[i] = 0
		}
	}()

	if !providerheaders.IsOpenRouter(cred.ProviderID, cred.BaseURL) {
		return false, nil
	}

	data, found, err := h.fetcher.fetchGeneration(ctx, cred.BaseURL, cred.APIKey, r.OpenRouterGenerationID)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	if data.NativeTokensPrompt == 0 && data.NativeTokensCompletion == 0 {
		return false, nil
	}

	providerID := r.ProviderID
	if providerID == "" {
		providerID = cred.ProviderID
	}
	cost, err := billing.EstimateCostUSD(nil, providerID, r.Model,
		int64(data.NativeTokensPrompt), int64(data.NativeTokensCompletion), 0)
	if err != nil {
		cost = 0
	}

	updates := map[string]any{
		"input_tokens":  data.NativeTokensPrompt,
		"output_tokens": data.NativeTokensCompletion,
	}
	if cost > 0 {
		updates["cost"] = cost
		updates["billing_cost_source"] = billing.CostSourceRegistry
	}
	if err := h.db.WithContext(ctx).Model(&model.Generation{}).
		Where("id = ? AND billed_at IS NULL", r.ID).
		Updates(updates).Error; err != nil {
		return false, fmt.Errorf("update generation %s: %w", r.ID, err)
	}
	return true, nil
}

type httpGenerationFetcher struct {
	client *http.Client
}

func (f *httpGenerationFetcher) fetchGeneration(ctx context.Context, baseURL string, apiKey []byte, id string) (openRouterGenerationData, bool, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/generation"

	var lastErr error
	for attempt := 0; ; attempt++ {
		data, found, err := f.attempt(ctx, endpoint, apiKey, id)
		if err == nil && found {
			return data, true, nil
		}
		if err != nil {
			lastErr = err
		}
		if attempt >= len(generationReconcileRetryDelays) {
			return openRouterGenerationData{}, false, lastErr
		}
		select {
		case <-time.After(generationReconcileRetryDelays[attempt]):
		case <-ctx.Done():
			return openRouterGenerationData{}, false, ctx.Err()
		}
	}
}

func (f *httpGenerationFetcher) attempt(ctx context.Context, endpoint string, apiKey []byte, id string) (openRouterGenerationData, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return openRouterGenerationData{}, false, err
	}
	q := req.URL.Query()
	q.Set("id", id)
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Authorization", "Bearer "+string(apiKey))
	req.Header.Set("Accept", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		return openRouterGenerationData{}, false, err
	}
	defer resp.Body.Close()

	// Not-yet-propagated records return 404; treat as a retryable "not found".
	if resp.StatusCode == http.StatusNotFound {
		return openRouterGenerationData{}, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return openRouterGenerationData{}, false, fmt.Errorf("generation endpoint status %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return openRouterGenerationData{}, false, err
	}
	var parsed openRouterGenerationResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return openRouterGenerationData{}, false, fmt.Errorf("decode generation response: %w", err)
	}
	if parsed.Data.ID == "" && parsed.Data.NativeTokensPrompt == 0 && parsed.Data.NativeTokensCompletion == 0 {
		return openRouterGenerationData{}, false, nil
	}
	return parsed.Data, true, nil
}
