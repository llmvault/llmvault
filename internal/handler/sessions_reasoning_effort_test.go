package handler

import (
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

func TestResolveSessionReasoningEffort(t *testing.T) {
	reqWith := func(effort string) createSessionRequest {
		return createSessionRequest{
			ModelDefinition: &sessionModelDefinitionRequest{ReasoningEffort: effort},
		}
	}

	tests := []struct {
		name         string
		req          createSessionRequest
		agentDefault string
		want         string
	}{
		{
			name:         "explicit wins over agent default",
			req:          reqWith("low"),
			agentDefault: "high",
			want:         "low",
		},
		{
			name:         "agent default used when request empty",
			req:          createSessionRequest{},
			agentDefault: "medium",
			want:         "medium",
		},
		{
			name:         "agent default used when request effort blank",
			req:          reqWith("  "),
			agentDefault: "high",
			want:         "high",
		},
		{
			name:         "backend default (low) when neither set",
			req:          createSessionRequest{},
			agentDefault: "",
			want:         model.DefaultReasoningEffort,
		},
		{
			name:         "invalid explicit falls back to agent default",
			req:          reqWith("extreme"),
			agentDefault: "medium",
			want:         "medium",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := model.Agent{DefaultReasoningEffort: tt.agentDefault}
			if got := resolveSessionReasoningEffort(tt.req, agent); got != tt.want {
				t.Fatalf("resolveSessionReasoningEffort = %q, want %q", got, tt.want)
			}
		})
	}
}
