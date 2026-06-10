package middleware

import (
	"context"
	"net/http"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/token"
)

// TokenAuth returns middleware that authenticates requests using sandbox proxy tokens (JWTs).
//
// It extracts the token from whichever auth scheme the LLM provider uses:
//
//   - Authorization: Bearer ptok_...  (OpenAI, Groq, Mistral, etc.)
//   - x-api-key: ptok_...            (Anthropic)
//   - api-key: ptok_...              (Azure)
//   - ?key=ptok_...                  (Google, query parameter)
//
// The JWT is validated, then checked against the database for revocation.
// If valid, TokenClaims are placed on the request context.
type ExpiredProxyTokenHandler interface {
	HandleExpiredProxyToken(context.Context, model.Token) error
}

type tokenAuthConfig struct {
	expiredProxyTokenHandler ExpiredProxyTokenHandler
}

type TokenAuthOption func(*tokenAuthConfig)

func WithExpiredProxyTokenHandler(handler ExpiredProxyTokenHandler) TokenAuthOption {
	return func(cfg *tokenAuthConfig) {
		cfg.expiredProxyTokenHandler = handler
	}
}

func TokenAuth(signingKey []byte, db *gorm.DB, opts ...TokenAuthOption) func(http.Handler) http.Handler {
	cfg := tokenAuthConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rawToken := extractProxyToken(r)
			if rawToken == "" {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authorization"})
				return
			}

			// Strip the ptok_ prefix if present
			jwtString, _ := strings.CutPrefix(rawToken, "ptok_")

			claims, reason, err := token.ValidateDetailed(signingKey, jwtString)
			if err != nil {
				if reason == token.ValidationExpired {
					maybeHandleExpiredProxyToken(r.Context(), db, cfg.expiredProxyTokenHandler, claims)
				}
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired token"})
				return
			}

			// Check if the token has been revoked
			var tokenRecord model.Token
			result := db.Where("jti = ?", claims.ID).First(&tokenRecord)
			if result.Error != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "token not found"})
				return
			}

			if tokenRecord.RevokedAt != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "token has been revoked"})
				return
			}

			tc := &TokenClaims{
				OrgID:        claims.OrgID,
				CredentialID: claims.CredentialID,
				JTI:          claims.ID,
				TokenType:    tokenMetaString(tokenRecord.Meta, "type"),
				IsSystem:     claims.IsSystem,
			}

			next.ServeHTTP(w, WithClaims(r, tc))
		})
	}
}

func maybeHandleExpiredProxyToken(ctx context.Context, db *gorm.DB, handler ExpiredProxyTokenHandler, claims *token.Claims) {
	if db == nil || handler == nil || claims == nil || strings.TrimSpace(claims.ID) == "" {
		return
	}
	var tokenRecord model.Token
	if err := db.WithContext(ctx).Where("jti = ?", claims.ID).First(&tokenRecord).Error; err != nil {
		return
	}
	if tokenRecord.RevokedAt != nil {
		return
	}
	if err := handler.HandleExpiredProxyToken(ctx, tokenRecord); err != nil {
		logging.Capture(ctx, err)
	}
}

// extractProxyToken extracts the proxy token from the request, checking
// all auth schemes that LLM providers use:
//
//  1. Authorization: Bearer ptok_...  (OpenAI and compatible)
//  2. x-api-key: ptok_...            (Anthropic)
//  3. api-key: ptok_...              (Azure)
//  4. ?key=ptok_...                  (Google query param)
func extractProxyToken(r *http.Request) string {
	// 1. Authorization: Bearer
	if auth := r.Header.Get("Authorization"); auth != "" {
		parts := strings.SplitN(auth, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return parts[1]
		}
	}

	// 2. x-api-key (Anthropic)
	if key := r.Header.Get("x-api-key"); key != "" {
		return key
	}

	// 3. api-key (Azure)
	if key := r.Header.Get("api-key"); key != "" {
		return key
	}

	// 4. ?key= query parameter (Google)
	if key := r.URL.Query().Get("key"); key != "" {
		return key
	}

	return ""
}

func tokenMetaString(meta model.JSON, key string) string {
	if value, ok := meta[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}
