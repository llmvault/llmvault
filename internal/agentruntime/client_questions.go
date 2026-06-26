package agentruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type QuestionAnswerValue struct {
	Answers []string `json:"answers"`
	Other   *string  `json:"other,omitempty"`
}

type QuestionAnswerPayload struct {
	Answers         map[string]QuestionAnswerValue `json:"answers"`
	User            *string                        `json:"user,omitempty"`
	UserDisplayName *string                        `json:"user_display_name,omitempty"`
}

type QuestionAnswerResponse struct {
	SessionID         string `json:"session_id"`
	QuestionRequestID string `json:"question_request_id"`
	State             string `json:"state"`
}

func (c *Client) PostQuestionAnswer(ctx context.Context, sessionID, questionRequestID string, body QuestionAnswerPayload) (*QuestionAnswerResponse, error) {
	sessionID = strings.TrimSpace(sessionID)
	questionRequestID = strings.TrimSpace(questionRequestID)
	if sessionID == "" {
		return nil, fmt.Errorf("post question answer: session_id is required")
	}
	if questionRequestID == "" {
		return nil, fmt.Errorf("post question answer: question_request_id is required")
	}
	path := "/sessions/" + url.PathEscape(sessionID) + "/questions/" + url.PathEscape(questionRequestID) + "/answer"
	resp, err := c.do(ctx, http.MethodPost, path, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, &HTTPStatusError{Op: "post question answer", StatusCode: resp.StatusCode, Status: resp.Status, Body: strings.TrimSpace(string(raw))}
	}
	var out QuestionAnswerResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode question answer response: %w", err)
	}
	return &out, nil
}
