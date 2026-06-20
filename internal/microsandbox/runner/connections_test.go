package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCountTCPFileCountsActiveConnectionsOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tcp")
	if err := os.WriteFile(path, []byte(`  sl  local_address rem_address   st
   0: 0100007F:A801 0100007F:9C40 01
   1: 0100007F:A801 00000000:0000 0A
   2: 0100007F:A802 0100007F:9C41 06
   3: 0100007F:A803 0100007F:9C42 08
`), 0o600); err != nil {
		t.Fatal(err)
	}
	counts := map[int]int{}
	countTCPFile(path, counts)
	if counts[43009] != 1 {
		t.Fatalf("established count = %d, want 1", counts[43009])
	}
	if counts[43010] != 0 {
		t.Fatalf("time-wait count = %d, want 0", counts[43010])
	}
	if counts[43011] != 1 {
		t.Fatalf("close-wait count = %d, want 1", counts[43011])
	}
}
