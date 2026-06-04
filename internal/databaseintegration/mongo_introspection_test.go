package databaseintegration

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestCollectMongoFieldPathsInfersNestedDocumentFields(t *testing.T) {
	fields := map[string]string{}
	collectMongoFieldPaths("", bson.M{
		"_id":   "user-123",
		"email": "foo@example.com",
		"profile": bson.M{
			"name": "Ada",
			"address": bson.M{
				"city": "Lagos",
			},
		},
		"tags": bson.A{"customer"},
	}, fields, 0)

	got := mongoFieldMapToList(fields)
	want := []MongoFieldInfo{
		{Path: "_id", Type: "string"},
		{Path: "email", Type: "string"},
		{Path: "profile", Type: "object"},
		{Path: "profile.address", Type: "object"},
		{Path: "profile.address.city", Type: "string"},
		{Path: "profile.name", Type: "string"},
		{Path: "tags", Type: "array"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d fields, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("field %d got %#v, want %#v", i, got[i], want[i])
		}
	}
}
