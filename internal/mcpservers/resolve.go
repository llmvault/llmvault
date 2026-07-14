package mcpservers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/access"
	"github.com/usehivy/hivy/internal/model"
)

// ResolveForRuntime resolves team inheritance, direct agent grants, disabled
// overrides, and (only when explicitly requested) the actor's personal
// attachments. Missing user/service authorizations omit only that server;
// corrupt credentials and failed token refreshes are returned as errors.
func (s *Service) ResolveForRuntime(ctx context.Context, orgID, agentID, teamID uuid.UUID, actorUserID *uuid.UUID, includePersonal bool) ([]RuntimeServer, error) {
	actualTeamID, err := s.agentTeam(ctx, orgID, agentID)
	if err != nil {
		return nil, err
	}
	if teamID != uuid.Nil && teamID != actualTeamID {
		return nil, ErrAgentNotFound
	}
	teamID = actualTeamID
	// Re-evaluate the human authorization on every compile. A future scheduled
	// run must not retain a former org/team member's personal credentials.
	if actorUserID != nil {
		actor, resolveErr := access.Resolve(ctx, s.db, orgID, actorUserID.String())
		if resolveErr != nil {
			actorUserID = nil
			includePersonal = false
		} else {
			allowed, accessErr := actor.CanManageTeamResource(ctx, s.db, teamID)
			if accessErr != nil {
				return nil, accessErr
			}
			if !allowed {
				actorUserID = nil
				includePersonal = false
			}
		}
	}
	servers := []model.MCPServer{}
	err = s.db.WithContext(ctx).Table("mcp_servers ms").Select("ms.*").
		Where("ms.org_id = ? AND ms.scope = ? AND ms.status = ?", orgID, model.MCPServerScopeOrg, model.MCPServerStatusActive).
		Where(`(
			EXISTS (SELECT 1 FROM team_mcp_servers tms WHERE tms.org_id = ms.org_id AND tms.team_id = ? AND tms.mcp_server_id = ms.id)
			OR EXISTS (SELECT 1 FROM agent_mcp_servers ams WHERE ams.org_id = ms.org_id AND ams.agent_id = ? AND ams.mcp_server_id = ms.id AND ams.state = ?)
		)`, teamID, agentID, model.MCPAgentGrantEnabled).
		Where(`NOT EXISTS (
			SELECT 1 FROM agent_mcp_servers ams WHERE ams.org_id = ms.org_id AND ams.agent_id = ? AND ams.mcp_server_id = ms.id AND ams.state = ?
		)`, agentID, model.MCPAgentGrantDisabled).
		Order("ms.name ASC, ms.id ASC").Scan(&servers).Error
	if err != nil {
		return nil, fmt.Errorf("resolve organization mcp servers: %w", err)
	}
	if includePersonal && actorUserID != nil {
		personal, err := s.PersonalAgentServers(ctx, orgID, *actorUserID, agentID)
		if err != nil {
			return nil, err
		}
		for _, server := range personal {
			if server.Status == model.MCPServerStatusActive {
				servers = append(servers, server)
			}
		}
	}
	result := make([]RuntimeServer, 0, len(servers))
	usedNames := map[string]bool{}
	for _, server := range servers {
		runtimeServer, usable, err := s.runtimeServer(ctx, server, actorUserID)
		if err != nil {
			return nil, fmt.Errorf("resolve credentials for MCP server %s: %w", server.ID, err)
		}
		if !usable {
			continue
		}
		name := server.Slug
		if usedNames[name] {
			name = name + "-" + server.ID.String()[:8]
		}
		usedNames[name] = true
		runtimeServer.Name = name
		result = append(result, runtimeServer)
	}
	return result, nil
}

func (s *Service) runtimeServer(ctx context.Context, server model.MCPServer, actorUserID *uuid.UUID) (RuntimeServer, bool, error) {
	result := RuntimeServer{ID: server.ID, Scope: server.Scope, URL: server.URL, Transport: server.Transport, Headers: map[string]string{}}
	if server.AuthType == model.MCPAuthTypeNone {
		return result, true, nil
	}
	authorization, err := s.selectAuthorization(ctx, server, actorUserID)
	if errors.Is(err, ErrAuthorizationNotFound) {
		return RuntimeServer{}, false, nil
	}
	if err != nil {
		return RuntimeServer{}, false, err
	}
	envelope, err := s.decryptEnvelope(authorization.CredentialsEncrypted)
	if err != nil {
		return RuntimeServer{}, false, err
	}
	if server.AuthType == model.MCPAuthTypeOAuthAuthorizationCode || server.AuthType == model.MCPAuthTypeOAuthClientCredentials {
		authorization, envelope, err = s.ensureAccessToken(ctx, server, *authorization)
		if errors.Is(err, ErrAuthorizationNotFound) {
			return RuntimeServer{}, false, nil
		}
		if err != nil {
			return RuntimeServer{}, false, err
		}
	}
	switch server.AuthType {
	case model.MCPAuthTypeStaticBearer:
		if envelope.BearerToken == "" {
			return RuntimeServer{}, false, nil
		}
		result.Headers["Authorization"] = "Bearer " + envelope.BearerToken
	case model.MCPAuthTypeStaticHeader:
		if envelope.HeaderValue == "" {
			return RuntimeServer{}, false, nil
		}
		result.Headers[http.CanonicalHeaderKey(server.HeaderName)] = envelope.HeaderValue
	case model.MCPAuthTypeOAuthAuthorizationCode, model.MCPAuthTypeOAuthClientCredentials:
		if envelope.AccessToken == "" {
			return RuntimeServer{}, false, nil
		}
		tokenType := strings.TrimSpace(authorization.TokenType)
		if tokenType == "" {
			tokenType = "Bearer"
		}
		result.Headers["Authorization"] = tokenType + " " + envelope.AccessToken
	default:
		return RuntimeServer{}, false, validationErrorf("unsupported auth_type")
	}
	return result, true, nil
}

func (s *Service) selectAuthorization(ctx context.Context, server model.MCPServer, actorUserID *uuid.UUID) (*model.MCPAuthorization, error) {
	loadUser := func() (*model.MCPAuthorization, error) {
		if actorUserID == nil {
			return nil, ErrAuthorizationNotFound
		}
		return s.getAuthorization(ctx, server.OrgID, server.ID, model.MCPPrincipalUser, *actorUserID)
	}
	loadService := func() (*model.MCPAuthorization, error) {
		return s.getAuthorization(ctx, server.OrgID, server.ID, model.MCPPrincipalOrgService, uuid.Nil)
	}
	if server.Scope == model.MCPServerScopePersonal {
		return loadUser()
	}
	switch server.AuthorizationPolicy {
	case model.MCPAuthorizationPolicyUserRequired:
		return loadUser()
	case model.MCPAuthorizationPolicyServiceRequired:
		return loadService()
	case model.MCPAuthorizationPolicyPreferUser:
		if row, err := loadUser(); err == nil {
			return row, nil
		} else if !errors.Is(err, ErrAuthorizationNotFound) {
			return nil, err
		}
		return loadService()
	case model.MCPAuthorizationPolicyPreferService:
		if row, err := loadService(); err == nil {
			return row, nil
		} else if !errors.Is(err, ErrAuthorizationNotFound) {
			return nil, err
		}
		return loadUser()
	default:
		return nil, ErrAuthorizationNotFound
	}
}
