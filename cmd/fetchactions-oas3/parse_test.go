package main

import "testing"

func TestParseSpecOperationSelectorsOnlyGenerateSelectedReadOperations(t *testing.T) {
	t.Parallel()

	spec := []byte(`
openapi: 3.1.0
info:
  title: Test API
  version: 1.0.0
paths:
  /widgets/:
    get:
      operationId: widgets_list
      responses:
        '200':
          description: OK
    post:
      operationId: widgets_create
      responses:
        '201':
          description: Created
  /widgets/{id}/:
    get:
      operationId: widgets_retrieve
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          description: OK
  /unselected/:
    get:
      operationId: unselected_list
      responses:
        '200':
          description: OK
`)

	result, err := parseSpec(spec, ServiceConfig{OperationSelectors: []OperationSelector{
		{Method: "GET", Path: "/widgets/"},
		{Method: "GET", Path: "/widgets/{id}/"},
	}})
	if err != nil {
		t.Fatalf("parseSpec() error = %v", err)
	}

	if len(result.Actions) != 2 {
		t.Fatalf("generated %d actions, want 2: %#v", len(result.Actions), result.Actions)
	}

	for _, actionKey := range []string{"widgets_list", "widgets_retrieve"} {
		action, ok := result.Actions[actionKey]
		if !ok {
			t.Errorf("missing selected action %q", actionKey)
			continue
		}
		if action.Access != "read" {
			t.Errorf("%s access = %q, want read", actionKey, action.Access)
		}
		if action.Execution == nil || action.Execution.Method != "GET" {
			t.Errorf("%s execution = %#v, want GET direct proxy execution", actionKey, action.Execution)
		}
	}

	for _, unexpected := range []string{"widgets_create", "unselected_list"} {
		if _, ok := result.Actions[unexpected]; ok {
			t.Errorf("generated unselected action %q", unexpected)
		}
	}
}
