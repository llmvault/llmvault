package databaseintegration

import "testing"

func TestValidateMongoCommand_AllowsFindAndAppliesLimit(t *testing.T) {
	cmd, err := ValidateMongoCommand([]byte(`{"find":"users","filter":{"active":true}}`), Policy{
		AllowedCollections: []string{"users"},
		MaxRows:            25,
	})
	if err != nil {
		t.Fatalf("ValidateMongoCommand returned error: %v", err)
	}
	if cmd["limit"] != 25 {
		t.Fatalf("limit = %#v, want 25", cmd["limit"])
	}
}

func TestValidateMongoCommand_DeniesDelete(t *testing.T) {
	_, err := ValidateMongoCommand([]byte(`{"delete":"users","deletes":[]}`), Policy{})
	if err == nil {
		t.Fatal("expected delete command to be denied")
	}
}

func TestValidateMongoCommand_DeniesCollectionOutsidePolicy(t *testing.T) {
	_, err := ValidateMongoCommand([]byte(`{"find":"payments"}`), Policy{AllowedCollections: []string{"users"}})
	if err == nil {
		t.Fatal("expected collection outside policy to be denied")
	}
}
