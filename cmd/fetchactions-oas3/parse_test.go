package main

import "testing"

func TestCuratedServiceActionCounts(t *testing.T) {
	t.Parallel()

	expected := map[string]int{"datadog": 50, "hubspot": 50, "resend": 50}
	found := make(map[string]bool, len(expected))
	for _, service := range AllServices() {
		want, ok := expected[service.Name]
		if !ok {
			continue
		}
		found[service.Name] = true
		if len(service.OperationSelectors) != want {
			t.Errorf("%s has %d curated operations, want %d", service.Name, len(service.OperationSelectors), want)
		}
		seen := make(map[OperationSelector]bool, len(service.OperationSelectors))
		for _, selector := range service.OperationSelectors {
			if seen[selector] {
				t.Errorf("%s repeats %s %s", service.Name, selector.Method, selector.Path)
			}
			seen[selector] = true
		}
	}
	for service := range expected {
		if !found[service] {
			t.Errorf("missing %s service configuration", service)
		}
	}
}

func TestNamespaceParseResultSchemas(t *testing.T) {
	t.Parallel()

	result := &ParseResult{
		Actions: map[string]ActionDef{
			"list_widgets": {ResponseSchema: "WidgetList"},
		},
		Schemas: map[string]FlatSchema{
			"WidgetList": {
				Type:  "array",
				Items: &FlatSchemaRef{Ref: "Widget"},
			},
			"Widget": {
				Type: "object",
				Properties: map[string]SchemaProperty{
					"owner": {Type: "object", SchemaRef: "Owner"},
				},
			},
			"Owner": {Type: "object"},
		},
	}

	namespaceParseResultSchemas(result, "source_one")
	if result.Actions["list_widgets"].ResponseSchema != "source_one_WidgetList" {
		t.Errorf("response schema = %q", result.Actions["list_widgets"].ResponseSchema)
	}
	if _, ok := result.Schemas["Widget"]; ok {
		t.Error("un-namespaced schema remains")
	}
	if got := result.Schemas["source_one_WidgetList"].Items.Ref; got != "source_one_Widget" {
		t.Errorf("item ref = %q", got)
	}
	if got := result.Schemas["source_one_Widget"].Properties["owner"].SchemaRef; got != "source_one_Owner" {
		t.Errorf("property ref = %q", got)
	}
}

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
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                name:
                  type: string
          multipart/form-data:
            schema:
              type: object
              properties:
                attachment:
                  type: string
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
		{Method: "POST", Path: "/widgets/"},
		{Method: "GET", Path: "/widgets/{id}/"},
	}})
	if err != nil {
		t.Fatalf("parseSpec() error = %v", err)
	}

	if len(result.Actions) != 3 {
		t.Fatalf("generated %d actions, want 3: %#v", len(result.Actions), result.Actions)
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

	created, ok := result.Actions["widgets_create"]
	if !ok {
		t.Error("missing selected write action widgets_create")
	} else {
		if created.Access != "write" {
			t.Errorf("widgets_create access = %q, want write", created.Access)
		}
		if created.Execution == nil || created.Execution.Method != "POST" {
			t.Errorf("widgets_create execution = %#v, want POST direct proxy execution", created.Execution)
		}
	}

	for _, unexpected := range []string{"unselected_list"} {
		if _, ok := result.Actions[unexpected]; ok {
			t.Errorf("generated unselected action %q", unexpected)
		}
	}
}
