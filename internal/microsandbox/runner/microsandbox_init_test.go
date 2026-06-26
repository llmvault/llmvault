package runner

import (
	"testing"

	microsandbox "github.com/superradcompany/microsandbox/sdk/go"
)

func TestSandboxInitOptionAuto(t *testing.T) {
	opt, ok := sandboxInitOption(&SandboxInitConfig{Cmd: "auto"})
	if !ok {
		t.Fatal("auto init option was not produced")
	}
	var cfg microsandbox.SandboxConfig
	opt(&cfg)
	if cfg.Init == nil || cfg.Init.Cmd != "auto" {
		t.Fatalf("init = %+v, want auto", cfg.Init)
	}
}

func TestSandboxInitOptionCommand(t *testing.T) {
	opt, ok := sandboxInitOption(&SandboxInitConfig{
		Cmd:  "/sbin/init",
		Args: []string{"--unit=multi-user.target"},
		Env:  map[string]string{"FOO": "bar"},
	})
	if !ok {
		t.Fatal("command init option was not produced")
	}
	var cfg microsandbox.SandboxConfig
	opt(&cfg)
	if cfg.Init == nil || cfg.Init.Cmd != "/sbin/init" {
		t.Fatalf("init = %+v, want /sbin/init", cfg.Init)
	}
	if len(cfg.Init.Args) != 1 || cfg.Init.Args[0] != "--unit=multi-user.target" {
		t.Fatalf("init args = %v", cfg.Init.Args)
	}
	if cfg.Init.Env["FOO"] != "bar" {
		t.Fatalf("init env = %+v", cfg.Init.Env)
	}
}
