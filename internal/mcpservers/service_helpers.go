package mcpservers

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func normalizeEndpointURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return "", validationErrorf("url must be an absolute HTTP(S) URL without credentials or a fragment")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", validationErrorf("url must use http or https")
	}
	if parsed.Scheme == "http" && !isLoopbackHost(parsed.Hostname()) {
		return "", validationErrorf("non-local MCP server URLs must use https")
	}
	return parsed.String(), nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func validateHeaderName(name string) error {
	if name == "" || !validHTTPToken(name) {
		return validationErrorf("header_name must be a valid HTTP header name")
	}
	switch strings.ToLower(name) {
	case "authorization", "host", "content-length", "connection", "transfer-encoding", "proxy-authorization", "mcp-session-id", "mcp-protocol-version":
		return validationErrorf("header_name is reserved")
	}
	return nil
}

func validHTTPToken(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r <= 32 || r >= 127 || strings.ContainsRune("()<>@,;:\\\"/[]?={}\t", r) {
			return false
		}
	}
	return true
}

func validAuthType(value string) bool {
	switch value {
	case model.MCPAuthTypeNone, model.MCPAuthTypeStaticBearer, model.MCPAuthTypeStaticHeader,
		model.MCPAuthTypeOAuthAuthorizationCode, model.MCPAuthTypeOAuthClientCredentials:
		return true
	default:
		return false
	}
}

func validateAuthorizationPolicy(scope, authType, policy string) error {
	valid := policy == model.MCPAuthorizationPolicyNone || policy == model.MCPAuthorizationPolicyUserRequired ||
		policy == model.MCPAuthorizationPolicyServiceRequired || policy == model.MCPAuthorizationPolicyPreferUser ||
		policy == model.MCPAuthorizationPolicyPreferService
	if !valid {
		return validationErrorf("unsupported authorization_policy")
	}
	if authType == model.MCPAuthTypeNone && policy != model.MCPAuthorizationPolicyNone {
		return validationErrorf("authorization_policy must be none when auth_type is none")
	}
	if authType != model.MCPAuthTypeNone && policy == model.MCPAuthorizationPolicyNone {
		return validationErrorf("authorization_policy cannot be none when authentication is required")
	}
	if scope == model.MCPServerScopePersonal && policy != model.MCPAuthorizationPolicyUserRequired && authType != model.MCPAuthTypeNone {
		return validationErrorf("personal MCP servers must use user_required authorization")
	}
	if authType == model.MCPAuthTypeOAuthClientCredentials && policy == model.MCPAuthorizationPolicyUserRequired {
		return validationErrorf("client credentials require an organization service authorization")
	}
	return nil
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	dash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if b.Len() > 0 && !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func ownerOrActor(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

func duplicateKey(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func hashState(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

func sortedUnique(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func (s *Service) encryptEnvelope(value credentialEnvelope) ([]byte, error) {
	if s.encKey == nil {
		return nil, fmt.Errorf("mcp servers: encryption key is not configured")
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode mcp credentials: %w", err)
	}
	encrypted, err := s.encKey.Encrypt(raw)
	if err != nil {
		return nil, fmt.Errorf("encrypt mcp credentials: %w", err)
	}
	return encrypted, nil
}

func (s *Service) decryptEnvelope(encrypted []byte) (credentialEnvelope, error) {
	if s.encKey == nil {
		return credentialEnvelope{}, fmt.Errorf("mcp servers: encryption key is not configured")
	}
	raw, err := s.encKey.Decrypt(encrypted)
	if err != nil {
		return credentialEnvelope{}, fmt.Errorf("decrypt mcp credentials: %w", err)
	}
	var value credentialEnvelope
	if err := json.Unmarshal(raw, &value); err != nil {
		return credentialEnvelope{}, fmt.Errorf("decode mcp credentials: %w", err)
	}
	return value, nil
}
