package runner

import (
	"strings"

	microsandbox "github.com/superradcompany/microsandbox/sdk/go"
)

func sandboxInitOption(init *SandboxInitConfig) (microsandbox.SandboxOption, bool) {
	if init == nil {
		return nil, false
	}
	cmd := strings.TrimSpace(init.Cmd)
	if cmd == "" {
		return nil, false
	}
	if cmd == "auto" {
		return microsandbox.WithInit(microsandbox.Init.Auto()), true
	}
	return microsandbox.WithInit(microsandbox.Init.Cmd(cmd, microsandbox.InitOptions{
		Args: init.Args,
		Env:  init.Env,
	})), true
}
