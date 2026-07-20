package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadHostPressureFiles(t *testing.T) {
	dir := t.TempDir()
	statPath := filepath.Join(dir, "stat")
	loadPath := filepath.Join(dir, "loadavg")
	if err := os.WriteFile(statPath, []byte("cpu  100 2 30 400 20 3 4 5 0 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(loadPath, []byte("2.75 1.00 0.50 7/321 999\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	times, ok := readCPUTimes(statPath)
	if !ok {
		t.Fatal("expected CPU times")
	}
	if times.total != 564 || times.idle != 420 {
		t.Fatalf("cpu times=%+v want total=564 idle=420", times)
	}
	load1, runnable := readLoadPressure(loadPath)
	if load1 != 2.75 || runnable != 7 {
		t.Fatalf("load=%v runnable=%d want 2.75/7", load1, runnable)
	}
}
