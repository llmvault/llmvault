package tasks

import (
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/runtimeevents"
)

func TestSessionReflectionRequiresThreeHumanMessageEvents(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE session_events (
		id text PRIMARY KEY,
		session_id text NOT NULL,
		event_type text NOT NULL,
		durability text NOT NULL DEFAULT ''
	)`).Error; err != nil {
		t.Fatal(err)
	}

	sessionID := uuid.New()
	insert := func(eventType string) {
		t.Helper()
		if err := db.Exec(
			"INSERT INTO session_events (id, session_id, event_type, durability) VALUES (?, ?, ?, 'durable')",
			uuid.New(), sessionID, eventType,
		).Error; err != nil {
			t.Fatal(err)
		}
	}
	insert(runtimeevents.EventUserMessageReceived)
	insert(runtimeevents.EventFinal)
	insert(runtimeevents.EventUserMessageReceived)
	insert(runtimeevents.EventFinal)
	insert(runtimeevents.EventToolResult)

	eligible, err := sessionHasMinimumReflectionMessages(db, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if eligible {
		t.Fatal("two human messages plus agent and tool events must not trigger reflection")
	}

	insert(runtimeevents.EventUserMessageReceived)
	eligible, err = sessionHasMinimumReflectionMessages(db, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !eligible {
		t.Fatal("three human messages should trigger reflection")
	}
}
