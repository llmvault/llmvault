package sheets

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// newSameOrgChannel creates a second native channel in the fixture's org so
// tests can prove sheets in one channel are invisible from another.
func (f *sheetsFixture) newSameOrgChannel(t *testing.T) model.Channel {
	t.Helper()
	ch := model.Channel{ID: uuid.New(), OrgID: f.org.ID, TeamID: f.team.ID, Name: "sheets-ch-" + uuid.NewString(), DefaultAgentID: f.agent.ID}
	if err := f.db.Create(&ch).Error; err != nil {
		t.Fatalf("create same-org channel: %v", err)
	}
	return ch
}

// TestCreateSheetRequiresChannel pins that a sheet cannot be created without a
// channel — the channel is mandatory, not optional.
func TestCreateSheetRequiresChannel(t *testing.T) {
	db := connectSheetsTestDB(t)
	f := seedSheetsFixture(t, db)
	_, err := f.svc.CreateSheet(context.Background(), f.org.ID, CreateSheetRequest{Name: "No channel"}, Actor{AgentID: &f.agent.ID})
	if err == nil {
		t.Fatal("CreateSheet without a channel should error")
	}
}

// TestListSheetsScopedToChannel proves ListSheets returns only the queried
// channel's sheets, never sibling channels in the same org.
func TestListSheetsScopedToChannel(t *testing.T) {
	db := connectSheetsTestDB(t)
	f := seedSheetsFixture(t, db)
	ctx := context.Background()

	channelB := f.newSameOrgChannel(t)
	sheetB, err := f.svc.CreateSheet(ctx, f.org.ID, CreateSheetRequest{
		Name:  "Channel B Sheet " + uuid.NewString(),
		Pages: []PageSpec{{Name: "P"}},
	}, Actor{AgentID: &f.agent.ID, ChannelID: channelB.ID})
	if err != nil {
		t.Fatalf("create channel B sheet: %v", err)
	}

	// Channel A (fixture channel) lists the fixture sheet, not B's.
	aSheets, _, err := f.svc.ListSheets(ctx, f.org.ID, f.channel.ID, "", QueryLimitREST, "")
	if err != nil {
		t.Fatalf("list channel A: %v", err)
	}
	for _, s := range aSheets {
		if s.ID == sheetB.Sheet.ID {
			t.Fatal("channel A list leaked channel B's sheet")
		}
	}
	// Channel B lists only B's sheet.
	bSheets, _, err := f.svc.ListSheets(ctx, f.org.ID, channelB.ID, "", QueryLimitREST, "")
	if err != nil {
		t.Fatalf("list channel B: %v", err)
	}
	if len(bSheets) != 1 || bSheets[0].ID != sheetB.Sheet.ID {
		t.Fatalf("channel B list = %d sheets, want exactly B's", len(bSheets))
	}
}

// TestChannelGuardsRejectCrossChannel proves the by-ID guards treat a sheet in
// another channel as not-found.
func TestChannelGuardsRejectCrossChannel(t *testing.T) {
	db := connectSheetsTestDB(t)
	f := seedSheetsFixture(t, db)
	ctx := context.Background()

	// The fixture sheet lives in channel A; assert guards reject it under B.
	channelB := f.newSameOrgChannel(t)
	sheetA := f.sheet.Sheet.ID
	pageA := f.leads.Page.ID
	fieldA := f.leads.Fields[0].ID

	if err := f.svc.SheetInChannel(ctx, f.org.ID, channelB.ID, sheetA); err != ErrNotFound {
		t.Fatalf("SheetInChannel cross-channel = %v, want ErrNotFound", err)
	}
	if err := f.svc.PageInChannel(ctx, f.org.ID, channelB.ID, pageA); err != ErrNotFound {
		t.Fatalf("PageInChannel cross-channel = %v, want ErrNotFound", err)
	}
	if err := f.svc.FieldInChannel(ctx, f.org.ID, channelB.ID, fieldA); err != ErrNotFound {
		t.Fatalf("FieldInChannel cross-channel = %v, want ErrNotFound", err)
	}
	// And accepts them under their own channel A.
	if err := f.svc.SheetInChannel(ctx, f.org.ID, f.channel.ID, sheetA); err != nil {
		t.Fatalf("SheetInChannel same-channel = %v, want nil", err)
	}
}

// TestSheetToolsChannelIsolation proves an agent whose session is in channel A
// can neither see nor touch a sheet in channel B of the SAME org through the
// MCP tools, and that sheet_create stamps the agent's current channel.
func TestSheetToolsChannelIsolation(t *testing.T) {
	fixture, client := setupSheetTools(t)
	ctx := fixture.toolCtx() // session is in channel A (fixture.channel)

	channelB := fixture.newSameOrgChannel(t)
	sheetB, err := fixture.svc.CreateSheet(context.Background(), fixture.org.ID, CreateSheetRequest{
		Name:  "Channel B Sheet " + uuid.NewString(),
		Pages: []PageSpec{{Name: "P", Fields: []FieldSpec{{Name: "Name", Type: FieldTypeText}}}},
	}, Actor{AgentID: &fixture.agent.ID, ChannelID: channelB.ID})
	if err != nil {
		t.Fatalf("create channel B sheet: %v", err)
	}
	sheetBID := sheetB.Sheet.ID.String()
	pageBID := sheetB.Pages[0].Page.ID.String()

	// 1. sheet_list (session in A) never returns channel B's sheet.
	listed := callSheetTool(t, ctx, client, toolSheetList, map[string]any{})
	for _, item := range listed["sheets"].([]any) {
		if item.(map[string]any)["id"] == sheetBID {
			t.Fatalf("sheet_list leaked channel B's sheet %s", sheetBID)
		}
	}

	// 2. Every by-ID tool treats channel B's sheet/page as not found.
	assertSheetToolError(t, ctx, client, toolSheetDescribe, map[string]any{"sheet_id": sheetBID}, "not found")
	assertSheetToolError(t, ctx, client, toolRowsQuery, map[string]any{"page_id": pageBID}, "not found")
	assertSheetToolError(t, ctx, client, toolRowsWrite, map[string]any{
		"page_id": pageBID, "action": "insert", "rows": []map[string]any{{"data": map[string]any{}}},
	}, "not found")
	assertSheetToolError(t, ctx, client, toolSheetManage, map[string]any{
		"action": "rename_sheet", "sheet_id": sheetBID, "name": "Hijacked",
	}, "not found")
	assertSheetToolError(t, ctx, client, toolSheetOperations, map[string]any{
		"action": "list", "page_id": pageBID,
	}, "not found")

	// 3. sheet_create stamps the agent's current channel (A), not B.
	created := callSheetTool(t, ctx, client, toolSheetCreate, map[string]any{"name": "Made in A " + uuid.NewString()})
	newID := mustUUID(t, created["sheet"].(map[string]any)["id"].(string))
	var row model.Sheet
	if err := fixture.db.First(&row, "id = ?", newID).Error; err != nil {
		t.Fatalf("reload created sheet: %v", err)
	}
	if row.ChannelID != fixture.channel.ID {
		t.Fatalf("created sheet channel = %s, want current channel A %s", row.ChannelID, fixture.channel.ID)
	}
}
