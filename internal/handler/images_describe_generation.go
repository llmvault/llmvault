package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/oklog/ulid/v2"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/system"
)

func (h *ImageDescribeHandler) writeImageDescribeGeneration(ctx context.Context, cred *model.Credential, orgID uuid.UUID, userID string, res *system.CompletionResult) {
	if res == nil {
		return
	}
	gen := model.Generation{
		ID:             "gen_" + ulid.Make().String(),
		OrgID:          orgID,
		CredentialID:   cred.ID,
		TokenJTI:       "system:images.describe",
		ProviderID:     imageDescribeProviderID,
		Model:          imageDescribeCanonicalModel,
		RequestPath:    "/v1/images/describe",
		IsStreaming:    false,
		InputTokens:    res.Usage.InputTokens,
		OutputTokens:   res.Usage.OutputTokens,
		CachedTokens:   res.Usage.CachedTokens,
		UpstreamStatus: http.StatusOK,
		UserID:         userID,
		CreatedAt:      time.Now(),
	}
	if err := h.db.WithContext(ctx).Create(&gen).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "image describe generation row write failed", "error", err, "generation_id", gen.ID)
	}
}
