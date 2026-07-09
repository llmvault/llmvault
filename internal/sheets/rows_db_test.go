package sheets

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestRowLifecycleAndQuery(t *testing.T) {
	ctx := context.Background()
	db := connectSheetsTestDB(t)
	f := seedSheetsFixture(t, db)
	nameID := f.fieldByName(t, f.leads, "Name").ID
	scoreID := f.fieldByName(t, f.leads, "Score").ID
	statusID := f.fieldByName(t, f.leads, "Status").ID

	if f.leads.Page.DisplayFieldID == nil || *f.leads.Page.DisplayFieldID != nameID {
		t.Fatalf("display field should default to first text field, got %v", f.leads.Page.DisplayFieldID)
	}

	rows := f.insertLeads(t,
		map[string]any{nameID: "Acme", scoreID: "12", statusID: "new"},
		map[string]any{nameID: "Globex", scoreID: 99, statusID: "qualified"},
		map[string]any{nameID: "Initech", statusID: "new"},
		map[string]any{nameID: "Umbrella", statusID: "new"},
		map[string]any{nameID: "Hooli", statusID: "qualified"},
	)
	stored := f.reloadRow(t, rows[0].ID)
	if stored.Data[scoreID] != 12.0 {
		t.Fatalf("score should be coerced from string to number, got %#v", stored.Data[scoreID])
	}

	// Unknown field key rejected.
	var unknownField *UnknownFieldError
	_, err := f.svc.InsertRows(ctx, f.org.ID, f.leads.Page.ID, []RowInsert{
		{Data: map[string]any{"fld_zzz9999999": "x"}},
	}, MaxRowsPerWriteMCP, f.actor)
	if !errors.As(err, &unknownField) {
		t.Fatalf("unknown field insert error = %v", err)
	}

	// Filter + search + keyset pagination.
	result, err := f.svc.QueryRows(ctx, f.org.ID, f.leads.Page.ID, Query{
		Filter: &Filter{Field: statusID, Op: OpEq, Value: "new"},
		Limit:  2,
	}, QueryLimitMCP)
	if err != nil {
		t.Fatalf("query rows: %v", err)
	}
	if len(result.Rows) != 2 || result.NextCursor == "" {
		t.Fatalf("first page = %d rows cursor %q", len(result.Rows), result.NextCursor)
	}
	second, err := f.svc.QueryRows(ctx, f.org.ID, f.leads.Page.ID, Query{
		Filter: &Filter{Field: statusID, Op: OpEq, Value: "new"},
		Limit:  2,
		Cursor: result.NextCursor,
	}, QueryLimitMCP)
	if err != nil {
		t.Fatalf("query second page: %v", err)
	}
	if len(second.Rows) != 1 || second.NextCursor != "" {
		t.Fatalf("second page = %d rows cursor %q", len(second.Rows), second.NextCursor)
	}
	if second.Rows[0].ID == result.Rows[0].ID || second.Rows[0].ID == result.Rows[1].ID {
		t.Fatalf("cursor page repeated a row")
	}
	searched, err := f.svc.QueryRows(ctx, f.org.ID, f.leads.Page.ID, Query{Search: "globex"}, QueryLimitMCP)
	if err != nil {
		t.Fatalf("search rows: %v", err)
	}
	if len(searched.Rows) != 1 || searched.Rows[0].Data[nameID] != "Globex" {
		t.Fatalf("search returned %d rows: %#v", len(searched.Rows), searched.Rows)
	}

	// Partial merge: only sent keys change; nil clears.
	_, err = f.svc.UpdateRows(ctx, f.org.ID, f.leads.Page.ID, []RowUpdate{
		{ID: rows[0].ID, Data: map[string]any{scoreID: 50, statusID: nil}},
	}, MaxRowsPerWriteMCP, f.actor)
	if err != nil {
		t.Fatalf("update rows: %v", err)
	}
	updated := f.reloadRow(t, rows[0].ID)
	if updated.Data[nameID] != "Acme" || updated.Data[scoreID] != 50.0 {
		t.Fatalf("partial merge broke untouched/updated keys: %#v", updated.Data)
	}
	if _, exists := updated.Data[statusID]; exists {
		t.Fatalf("nil update should clear the cell: %#v", updated.Data)
	}

	// Soft delete.
	deleted, err := f.svc.DeleteRows(ctx, f.org.ID, f.leads.Page.ID, []uuid.UUID{rows[4].ID}, MaxRowsPerWriteMCP, f.actor)
	if err != nil || deleted != 1 {
		t.Fatalf("delete rows = (%d, %v)", deleted, err)
	}
	if archived := f.reloadRow(t, rows[4].ID); archived.ArchivedAt == nil {
		t.Fatalf("delete must archive, not remove")
	}

	// Org scoping: another org can see nothing through any read/write path.
	if _, err := f.svc.QueryRows(ctx, f.otherOrg.ID, f.leads.Page.ID, Query{}, QueryLimitMCP); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-org query error = %v, want ErrNotFound", err)
	}
	if _, err := f.svc.UpdateRows(ctx, f.otherOrg.ID, f.leads.Page.ID, []RowUpdate{{ID: rows[0].ID, Data: map[string]any{nameID: "pwned"}}}, MaxRowsPerWriteMCP, Actor{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-org update error = %v, want ErrNotFound", err)
	}
	if _, err := f.svc.GetSheet(ctx, f.otherOrg.ID, f.sheet.Sheet.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-org sheet read error = %v, want ErrNotFound", err)
	}

	// Batch limit.
	var limitErr *LimitError
	tooMany := make([]RowInsert, MaxRowsPerWriteMCP+1)
	for i := range tooMany {
		tooMany[i] = RowInsert{Data: map[string]any{nameID: "x"}}
	}
	if _, err := f.svc.InsertRows(ctx, f.org.ID, f.leads.Page.ID, tooMany, MaxRowsPerWriteMCP, f.actor); !errors.As(err, &limitErr) {
		t.Fatalf("oversized batch error = %v, want LimitError", err)
	}
}

func TestRelationAndAttachmentValidation(t *testing.T) {
	ctx := context.Background()
	db := connectSheetsTestDB(t)
	f := seedSheetsFixture(t, db)
	nameID := f.fieldByName(t, f.leads, "Name").ID
	companyNameID := f.fieldByName(t, f.companies, "Company").ID
	filesID := f.fieldByName(t, f.leads, "Files").ID

	relField := f.createRelationField(t, f.companies.Page.ID)
	companies, err := f.svc.InsertRows(ctx, f.org.ID, f.companies.Page.ID, []RowInsert{
		{Data: map[string]any{companyNameID: "Acme Corp"}},
	}, MaxRowsPerWriteMCP, f.actor)
	if err != nil {
		t.Fatalf("insert company: %v", err)
	}

	// Valid same-org, right-page link + hydration.
	leads := f.insertLeads(t, map[string]any{nameID: "Lead", relField.ID: []any{companies[0].ID.String()}})
	result, err := f.svc.QueryRows(ctx, f.org.ID, f.leads.Page.ID, Query{ResolveRelations: true}, QueryLimitMCP)
	if err != nil {
		t.Fatalf("query with resolve_relations: %v", err)
	}
	ref, ok := result.Relations[companies[0].ID.String()]
	if !ok || ref.Label != "Acme Corp" {
		t.Fatalf("relation hydration = %#v, want label from display field", result.Relations)
	}

	var relErr *RelationError
	// Wrong page: linking a leads row through a companies-targeted field.
	if _, err := f.svc.UpdateRows(ctx, f.org.ID, f.leads.Page.ID, []RowUpdate{
		{ID: leads[0].ID, Data: map[string]any{relField.ID: []any{leads[0].ID.String()}}},
	}, MaxRowsPerWriteMCP, f.actor); !errors.As(err, &relErr) {
		t.Fatalf("wrong-page link error = %v, want RelationError", err)
	}
	// Cross-org row id.
	if _, err := f.svc.InsertRows(ctx, f.org.ID, f.leads.Page.ID, []RowInsert{
		{Data: map[string]any{relField.ID: []any{f.otherRow.ID.String()}}},
	}, MaxRowsPerWriteMCP, f.actor); !errors.As(err, &relErr) {
		t.Fatalf("cross-org link error = %v, want RelationError", err)
	}
	// Cross-org target page at field-create time.
	if _, err := f.svc.CreateField(ctx, f.org.ID, f.leads.Page.ID, FieldSpec{
		Name: "Evil", Type: FieldTypeRelation,
		Options: map[string]any{"target_page_id": f.otherPage.Page.ID.String()},
	}, f.actor); !errors.As(err, &relErr) {
		t.Fatalf("cross-org target page error = %v, want RelationError", err)
	}
	// Defense in depth: even a tampered definition pointing cross-org is
	// rejected on write because the target page's org is re-checked.
	tamperedID, _ := NewFieldID()
	if err := db.Exec(
		`INSERT INTO sheet_fields (id, page_id, org_id, name, type, options, position) VALUES (?, ?, ?, 'Tampered', 'relation', ?, 99999)`,
		tamperedID, f.leads.Page.ID, f.org.ID, `{"target_page_id":"`+f.otherPage.Page.ID.String()+`"}`,
	).Error; err != nil {
		t.Fatalf("seed tampered field: %v", err)
	}
	if _, err := f.svc.InsertRows(ctx, f.org.ID, f.leads.Page.ID, []RowInsert{
		{Data: map[string]any{tamperedID: []any{f.otherRow.ID.String()}}},
	}, MaxRowsPerWriteMCP, f.actor); !errors.As(err, &relErr) {
		t.Fatalf("tampered-field link error = %v, want RelationError", err)
	}

	// Attachments: org-owned prefix accepted outright. Any pub/o/{orgID}/
	// key qualifies (not just sheets/attachments/ uploads) — the same
	// admission rule the download-url endpoint applies
	// (Service.AuthorizeObjectKeys). Drive keys of the org's OWN agents
	// (pub/e/{agentID}/…) are accepted too, via the batched agents lookup.
	ownKey := OrgAttachmentPrefix(f.org.ID) + "sheets/attachments/file.png"
	brandKey := OrgAttachmentPrefix(f.org.ID) + "brand-assets/logo.png"
	orgAgentDriveKey := "pub/e/" + f.agent.ID.String() + "/report.pdf"
	if _, err := f.svc.InsertRows(ctx, f.org.ID, f.leads.Page.ID, []RowInsert{
		{Data: map[string]any{filesID: []any{ownKey, brandKey, orgAgentDriveKey}}},
	}, MaxRowsPerWriteMCP, f.actor); err != nil {
		t.Fatalf("org-owned/org-agent attachment rejected: %v", err)
	}
	if err := f.svc.AuthorizeObjectKeys(ctx, f.org.ID, []string{ownKey, brandKey, orgAgentDriveKey}); err != nil {
		t.Fatalf("AuthorizeObjectKeys rejected accepted cell keys: %v", err)
	}

	// A foreign org's agent — its drive keys must stay unreachable from f.org.
	foreignAgent := model.Agent{ID: uuid.New(), OrgID: &f.otherOrg.ID, TeamID: f.otherTeam.ID, Name: "Foreign Agent " + uuid.NewString(), Model: "test", Status: "active"}
	if err := db.Create(&foreignAgent).Error; err != nil {
		t.Fatalf("seed foreign agent: %v", err)
	}

	var attErr *AttachmentError
	foreignKey := OrgAttachmentPrefix(f.otherOrg.ID) + "sheets/attachments/file.png"
	foreignAgentDriveKey := "pub/e/" + foreignAgent.ID.String() + "/report.pdf"
	missingAgentDriveKey := "pub/e/" + uuid.NewString() + "/report.pdf"
	notUUIDDriveKey := "pub/e/not-a-uuid/report.pdf"
	emptyDriveRemainder := "pub/e/" + f.agent.ID.String() + "/"
	driveTraversalKey := "pub/e/" + f.agent.ID.String() + "/../escape.pdf"
	traversalKey := OrgAttachmentPrefix(f.org.ID) + "sheets/attachments/../../escape.png"
	rejected := []string{
		foreignKey, foreignAgentDriveKey, missingAgentDriveKey, notUUIDDriveKey,
		emptyDriveRemainder, driveTraversalKey, traversalKey,
		"pub/o/file.png", "pub/e/", "other/prefix.png", OrgAttachmentPrefix(f.org.ID),
	}
	for _, key := range rejected {
		if _, err := f.svc.InsertRows(ctx, f.org.ID, f.leads.Page.ID, []RowInsert{
			{Data: map[string]any{filesID: []any{key}}},
		}, MaxRowsPerWriteMCP, f.actor); !errors.As(err, &attErr) {
			t.Fatalf("attachment key %q error = %v, want AttachmentError", key, err)
		}
		if err := f.svc.AuthorizeObjectKeys(ctx, f.org.ID, []string{key}); !errors.As(err, &attErr) {
			t.Fatalf("AuthorizeObjectKeys(%q) = %v, want AttachmentError", key, err)
		}
	}
}
