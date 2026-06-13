package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	sentrygo "github.com/getsentry/sentry-go"
	"github.com/google/uuid"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
)

const providerProxyDeniedMessage = "This action is currently denied by Hivy's provider proxy safety policy. Delete, remove, destroy, archive, and trash operations must be performed by a human directly in the provider dashboard."

type providerProxyPolicyContext struct {
	Provider      string
	OrgID         uuid.UUID
	CallerAgentID uuid.UUID
	AgentID       uuid.UUID
	ConnectionID  uuid.UUID
	Method        string
	Path          string
	Operation     string
}

type proxyPolicyDecision struct {
	Denied    bool
	Operation string
}

func readProxyBody(r *http.Request) ([]byte, error) {
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

func proxyRequestBodyFromBytes(method string, body []byte) io.Reader {
	if method == http.MethodGet || method == http.MethodHead {
		return nil
	}
	return bytes.NewReader(body)
}

func enforceProviderProxyPolicy(w http.ResponseWriter, ctx context.Context, eventCtx providerProxyPolicyContext, body []byte) bool {
	decision := providerProxyPolicyDecisionFor(eventCtx.Provider, eventCtx.Method, eventCtx.Path, body)
	if !decision.Denied {
		return false
	}
	eventCtx.Operation = decision.Operation
	captureProviderProxyPolicyDenial(ctx, eventCtx)
	writeJSON(w, http.StatusForbidden, map[string]string{
		"error":     providerProxyDeniedMessage,
		"operation": decision.Operation,
	})
	return true
}

func providerProxyPolicyDecisionFor(provider, method, path string, body []byte) proxyPolicyDecision {
	provider = strings.ToLower(strings.TrimSpace(provider))
	path = "/" + strings.TrimLeft(path, "/")
	if method == http.MethodDelete {
		return denyProxyOperation(method + " " + path)
	}

	switch provider {
	case "railway", "linear":
		return graphQLProxyPolicyDecision(body)
	case "slack":
		return slackProxyPolicyDecision(path)
	case "notion":
		return notionProxyPolicyDecision(method, path, body)
	case "vercel":
		return vercelProxyPolicyDecision(path, body)
	case "bugsink":
		return restPathProxyPolicyDecision(path, body)
	default:
		return restPathProxyPolicyDecision(path, body)
	}
}

func graphQLProxyPolicyDecision(body []byte) proxyPolicyDecision {
	operations, err := graphQLTopLevelMutationFields(body)
	if err != nil {
		if bytes.Contains(bytes.ToLower(body), []byte("mutation")) {
			return denyProxyOperation("uninspectable GraphQL mutation")
		}
		return proxyPolicyDecision{}
	}
	for _, operation := range operations {
		if destructiveOperationName(operation) {
			return denyProxyOperation(operation)
		}
	}
	return proxyPolicyDecision{}
}

func graphQLTopLevelMutationFields(body []byte) ([]string, error) {
	payloads, err := graphQLPayloads(body)
	if err != nil {
		return nil, err
	}
	var operations []string
	for _, payload := range payloads {
		query := strings.TrimSpace(payload.Query)
		if query == "" {
			continue
		}
		doc, err := parser.ParseQuery(&ast.Source{Input: query})
		if err != nil {
			return nil, err
		}
		for _, op := range doc.Operations {
			if op.Operation != ast.Mutation {
				continue
			}
			for _, child := range op.SelectionSet {
				field, ok := child.(*ast.Field)
				if ok && field.Name != "" {
					operations = append(operations, field.Name)
				}
			}
		}
	}
	return operations, nil
}

type graphQLPayload struct {
	Query string `json:"query"`
}

func graphQLPayloads(body []byte) ([]graphQLPayload, error) {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return nil, nil
	}
	if body[0] == '[' {
		var payloads []graphQLPayload
		return payloads, json.Unmarshal(body, &payloads)
	}
	var payload graphQLPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	return []graphQLPayload{payload}, nil
}

func slackProxyPolicyDecision(path string) proxyPolicyDecision {
	apiMethod := strings.Trim(path, "/")
	if destructiveOperationName(apiMethod) {
		return denyProxyOperation(apiMethod)
	}
	return proxyPolicyDecision{}
}

func notionProxyPolicyDecision(method, path string, body []byte) proxyPolicyDecision {
	if method == http.MethodPatch || method == http.MethodPost {
		if op, ok := destructiveJSONBodyOperation(body); ok {
			return denyProxyOperation(op)
		}
	}
	return restPathProxyPolicyDecision(path, body)
}

func vercelProxyPolicyDecision(path string, body []byte) proxyPolicyDecision {
	if op, ok := destructiveJSONBodyOperation(body); ok {
		return denyProxyOperation(op)
	}
	return restPathProxyPolicyDecision(path, body)
}

func restPathProxyPolicyDecision(path string, body []byte) proxyPolicyDecision {
	for _, segment := range strings.FieldsFunc(path, func(r rune) bool {
		return r == '/' || r == '-' || r == '_' || r == '.'
	}) {
		if destructiveOperationName(segment) {
			return denyProxyOperation(segment)
		}
	}
	if op, ok := destructiveJSONBodyOperation(body); ok {
		return denyProxyOperation(op)
	}
	return proxyPolicyDecision{}
}

func destructiveJSONBodyOperation(body []byte) (string, bool) {
	body = bytes.TrimSpace(body)
	if len(body) == 0 || !(body[0] == '{' || body[0] == '[') {
		return "", false
	}
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return "", false
	}
	return destructiveJSONValue(value)
}

func destructiveJSONValue(value any) (string, bool) {
	switch typed := value.(type) {
	case map[string]any:
		for key, val := range typed {
			if destructiveFlag(key, val) {
				return key, true
			}
			if strings.EqualFold(key, "operation") {
				if operation, ok := val.(string); ok && destructiveOperationName(operation) {
					return operation, true
				}
			}
			if op, ok := destructiveJSONValue(val); ok {
				return op, true
			}
		}
	case []any:
		for _, item := range typed {
			if op, ok := destructiveJSONValue(item); ok {
				return op, true
			}
		}
	}
	return "", false
}

func destructiveFlag(key string, value any) bool {
	boolValue, ok := value.(bool)
	if !ok || !boolValue {
		return false
	}
	switch strings.ToLower(key) {
	case "archived", "is_archived", "in_trash", "trashed":
		return true
	default:
		return false
	}
}

func destructiveOperationName(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	for _, token := range []string{"delete", "remove", "destroy", "archive", "trash"} {
		if lower == token || strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

func denyProxyOperation(operation string) proxyPolicyDecision {
	return proxyPolicyDecision{Denied: true, Operation: operation}
}

func captureProviderProxyPolicyDenial(ctx context.Context, eventCtx providerProxyPolicyContext) {
	hub := sentrygo.GetHubFromContext(ctx)
	if hub == nil {
		hub = sentrygo.CurrentHub()
	}
	hub.WithScope(func(scope *sentrygo.Scope) {
		scope.SetLevel(sentrygo.LevelWarning)
		scope.SetTag("provider_proxy_denied", "true")
		scope.SetTag("provider", eventCtx.Provider)
		scope.SetTag("http.method", eventCtx.Method)
		scope.SetTag("http.status_code", fmt.Sprintf("%d", http.StatusForbidden))
		if eventCtx.Path != "" {
			scope.SetTag("provider.path", eventCtx.Path)
		}
		if eventCtx.Operation != "" {
			scope.SetTag("provider.operation", eventCtx.Operation)
		}
		if eventCtx.OrgID != uuid.Nil {
			scope.SetTag("org_id", eventCtx.OrgID.String())
		}
		if eventCtx.CallerAgentID != uuid.Nil {
			scope.SetTag("caller_agent_id", eventCtx.CallerAgentID.String())
		}
		if eventCtx.AgentID != uuid.Nil {
			scope.SetTag("agent_id", eventCtx.AgentID.String())
		}
		if eventCtx.ConnectionID != uuid.Nil {
			scope.SetTag("connection_id", eventCtx.ConnectionID.String())
		}
		hub.CaptureException(errors.New("provider proxy denied destructive operation"))
	})
}
