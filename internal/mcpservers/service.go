// Package mcpservers owns the multi-tenant MCP control plane: remote server
// definitions, encrypted authorizations, team/agent provisioning, and typed
// runtime resolution. It is deliberately separate from internal/mcpserver,
// which serves Hivy's own MCP protocol endpoint.
package mcpservers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/netguard"
)

const maxOAuthResponseBytes = 1 << 20

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

const reservedRuntimeServerSlug = "hivy"

type Service struct {
	db          *gorm.DB
	encKey      *crypto.SymmetricKey
	httpClient  *http.Client
	callbackURL string
	now         func() time.Time
}

func NewService(db *gorm.DB, encKey *crypto.SymmetricKey, callbackURL string) *Service {
	return &Service{
		db:     db,
		encKey: encKey,
		httpClient: &http.Client{
			Transport: netguard.NewTransport(),
			Timeout:   15 * time.Second,
			CheckRedirect: func(request *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return errors.New("mcp servers: too many redirects")
				}
				_, err := normalizeEndpointURL(request.URL.String())
				return err
			},
		},
		callbackURL: strings.TrimSpace(callbackURL),
		now:         func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) WithHTTPClient(client *http.Client) *Service {
	if client != nil {
		s.httpClient = client
	}
	return s
}

func (s *Service) CreateServer(ctx context.Context, orgID, actorUserID uuid.UUID, params CreateServerParams) (*model.MCPServer, error) {
	server, err := normalizeServer(params, orgID, actorUserID)
	if err != nil {
		return nil, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.WithContext(ctx).Create(&server).Error; err != nil {
			if duplicateKey(err) {
				return ErrConflict
			}
			return fmt.Errorf("create mcp server: %w", err)
		}
		if params.Authorization == nil || server.AuthType == model.MCPAuthTypeNone {
			return nil
		}
		return s.upsertAuthorization(ctx, tx, server, actorUserID, *params.Authorization)
	})
	if err != nil {
		return nil, err
	}
	return &server, nil
}

func normalizeServer(params CreateServerParams, orgID, actorUserID uuid.UUID) (model.MCPServer, error) {
	scope := strings.TrimSpace(params.Scope)
	if scope != model.MCPServerScopePersonal && scope != model.MCPServerScopeOrg {
		return model.MCPServer{}, validationErrorf("scope must be personal or org")
	}
	name := strings.TrimSpace(params.Name)
	if name == "" || len(name) > 100 {
		return model.MCPServer{}, validationErrorf("name is required and must not exceed 100 characters")
	}
	slug := strings.TrimSpace(strings.ToLower(params.Slug))
	if slug == "" {
		slug = slugify(name)
	}
	if len(slug) > 80 || !slugPattern.MatchString(slug) {
		return model.MCPServer{}, validationErrorf("slug must contain lowercase letters, numbers, and single hyphens")
	}
	if slug == reservedRuntimeServerSlug {
		return model.MCPServer{}, validationErrorf("slug is reserved")
	}
	endpoint, err := normalizeEndpointURL(params.URL)
	if err != nil {
		return model.MCPServer{}, err
	}
	transport := strings.TrimSpace(params.Transport)
	if transport == "" {
		transport = model.MCPTransportStreamableHTTP
	}
	if transport != model.MCPTransportStreamableHTTP && transport != model.MCPTransportSSE {
		return model.MCPServer{}, validationErrorf("transport must be streamable_http or sse")
	}
	authType := strings.TrimSpace(params.AuthType)
	if authType == "" {
		authType = model.MCPAuthTypeNone
	}
	if !validAuthType(authType) {
		return model.MCPServer{}, validationErrorf("unsupported auth_type")
	}
	policy := strings.TrimSpace(params.AuthorizationPolicy)
	if policy == "" {
		if authType == model.MCPAuthTypeNone {
			policy = model.MCPAuthorizationPolicyNone
		} else if scope == model.MCPServerScopePersonal {
			policy = model.MCPAuthorizationPolicyUserRequired
		} else {
			policy = model.MCPAuthorizationPolicyServiceRequired
		}
	}
	if err := validateAuthorizationPolicy(scope, authType, policy); err != nil {
		return model.MCPServer{}, err
	}
	headerName := strings.TrimSpace(params.HeaderName)
	if authType == model.MCPAuthTypeStaticHeader {
		if err := validateHeaderName(headerName); err != nil {
			return model.MCPServer{}, err
		}
	} else {
		headerName = ""
	}
	owner := (*uuid.UUID)(nil)
	if scope == model.MCPServerScopePersonal {
		if actorUserID == uuid.Nil {
			return model.MCPServer{}, validationErrorf("personal MCP servers require a user")
		}
		owner = &actorUserID
	}
	return model.MCPServer{
		OrgID: orgID, Scope: scope, OwnerUserID: owner, Slug: slug, Name: name,
		Description: strings.TrimSpace(params.Description), URL: endpoint,
		Transport: transport, AuthType: authType, AuthorizationPolicy: policy,
		HeaderName: headerName, OAuthMetadata: encodeOAuthMetadata(params.OAuthMetadata),
		Status: model.MCPServerStatusActive, CreatedByUserID: ownerOrActor(actorUserID),
	}, nil
}
