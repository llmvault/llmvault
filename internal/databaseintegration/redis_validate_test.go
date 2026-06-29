package databaseintegration

import "testing"

func TestValidateRedisCommandAllowsReadOnlyJSONCommand(t *testing.T) {
	cmd, err := ValidateRedisCommand([]byte(`{"command":"GET","args":["user:1"]}`), Policy{
		AllowedKeys: []string{"user:*"},
		MaxRows:     25,
	})
	if err != nil {
		t.Fatalf("ValidateRedisCommand returned error: %v", err)
	}
	if cmd.Command != "GET" || len(cmd.Args) != 1 || cmd.Args[0] != "user:1" {
		t.Fatalf("command = %#v", cmd)
	}
}

func TestValidateRedisCommandDeniesWriteCommand(t *testing.T) {
	_, err := ValidateRedisCommand([]byte(`{"command":"SET","args":["user:1","Ada"]}`), Policy{})
	if err == nil {
		t.Fatal("expected SET command to be denied")
	}
}

func TestValidateRedisCommandDeniesKeyOutsidePolicy(t *testing.T) {
	_, err := ValidateRedisCommand([]byte(`["GET","payment:1"]`), Policy{AllowedKeys: []string{"user:*"}})
	if err == nil {
		t.Fatal("expected key outside policy to be denied")
	}
}

func TestValidateRedisCommandBoundsRangeCommands(t *testing.T) {
	_, err := ValidateRedisCommand([]byte(`{"command":"LRANGE","args":["events","0","-1"]}`), Policy{MaxRows: 25})
	if err == nil {
		t.Fatal("expected unbounded LRANGE command to be denied")
	}
}

func TestValidateRedisCommandAddsScanCount(t *testing.T) {
	cmd, err := ValidateRedisCommand([]byte(`{"command":"SCAN","args":["0"]}`), Policy{MaxRows: 25})
	if err != nil {
		t.Fatalf("ValidateRedisCommand returned error: %v", err)
	}
	if len(cmd.Args) != 3 || cmd.Args[1] != "COUNT" || cmd.Args[2] != "25" {
		t.Fatalf("SCAN args = %#v, want COUNT 25", cmd.Args)
	}
}
