package mcpservers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func (s *Service) fetchJSON(ctx context.Context, rawURL string, destination any) error {
	validated, err := normalizeEndpointURL(rawURL)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, validated, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	response, err := s.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.CopyN(io.Discard, response.Body, maxOAuthResponseBytes)
		return fmt.Errorf("metadata endpoint returned status %d", response.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(response.Body, maxOAuthResponseBytes)).Decode(destination)
}

func authorizationMetadataURL(issuer, wellKnownPath string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(issuer))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return "", validationErrorf("authorization server issuer is invalid")
	}
	issuerPath := strings.TrimSuffix(parsed.EscapedPath(), "/")
	parsed.Path = wellKnownPath + issuerPath
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func sameOAuthIssuer(expected, actual string) bool {
	left, leftErr := url.Parse(strings.TrimSpace(expected))
	right, rightErr := url.Parse(strings.TrimSpace(actual))
	if leftErr != nil || rightErr != nil || left.Host == "" || right.Host == "" {
		return false
	}
	return strings.EqualFold(left.Scheme, right.Scheme) &&
		strings.EqualFold(left.Hostname(), right.Hostname()) &&
		effectiveOAuthPort(left) == effectiveOAuthPort(right) &&
		left.EscapedPath() == right.EscapedPath() &&
		left.RawQuery == right.RawQuery
}

func effectiveOAuthPort(value *url.URL) string {
	if value.Port() != "" {
		return value.Port()
	}
	if strings.EqualFold(value.Scheme, "https") {
		return "443"
	}
	if strings.EqualFold(value.Scheme, "http") {
		return "80"
	}
	return ""
}

func protectedResourceMetadataURLs(resource string) []string {
	parsed, err := url.Parse(resource)
	if err != nil || parsed.Host == "" {
		return nil
	}
	resourcePath := strings.TrimSuffix(parsed.EscapedPath(), "/")
	parsed.Path = "/.well-known/oauth-protected-resource" + resourcePath
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	pathSpecific := parsed.String()
	parsed.Path = "/.well-known/oauth-protected-resource"
	root := parsed.String()
	if pathSpecific == root {
		return []string{root}
	}
	return []string{pathSpecific, root}
}

func validateOAuthMetadataEndpoints(metadata OAuthMetadata) error {
	for label, endpoint := range map[string]string{
		"authorization_endpoint":          metadata.AuthorizationEndpoint,
		"token_endpoint":                  metadata.TokenEndpoint,
		"registration_endpoint":           metadata.RegistrationEndpoint,
		"protected_resource_metadata_url": metadata.ProtectedResourceURL,
	} {
		if endpoint == "" {
			continue
		}
		if _, err := normalizeEndpointURL(endpoint); err != nil {
			return validationErrorf("%s is invalid or insecure", label)
		}
	}
	return nil
}

func bearerParameter(header, name string) string {
	header = strings.TrimSpace(header)
	index := strings.Index(strings.ToLower(header), "bearer ")
	if index < 0 {
		return ""
	}
	value := strings.TrimSpace(header[index+len("Bearer "):])
	for _, part := range strings.Split(value, ",") {
		key, raw, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && strings.EqualFold(strings.TrimSpace(key), name) {
			unquoted, err := strconv.Unquote(strings.TrimSpace(raw))
			if err == nil {
				return unquoted
			}
			return strings.Trim(strings.TrimSpace(raw), "\"")
		}
	}
	return ""
}

func randomURLToken(bytes int) (string, error) {
	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func safeRedirectAfter(value string) string {
	value = strings.TrimSpace(value)
	parsed, err := url.Parse(value)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" ||
		!strings.HasPrefix(parsed.Path, "/") || strings.HasPrefix(parsed.Path, "//") || strings.Contains(parsed.Path, "\\") {
		return ""
	}
	for _, r := range parsed.Path {
		if r < 0x20 || r == 0x7f {
			return ""
		}
	}
	return parsed.EscapedPath()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
