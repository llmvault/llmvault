package microsandbox

import (
	"strings"

	"github.com/usehivy/hivy/internal/sandbox"
)

type createSandboxResponse struct {
	ID     string `json:"ID"`
	Status string `json:"Status"`
}

type sandboxResponse struct {
	ID     string `json:"ID"`
	Status string `json:"Status"`
}

type templateResponse struct {
	ID           string `json:"ID"`
	Status       string `json:"Status"`
	Logs         string `json:"Logs"`
	ErrorMessage string `json:"ErrorMessage"`
}

type templateBuildEvent struct {
	Type    string `json:"type"`
	Status  string `json:"status,omitempty"`
	Message string `json:"message,omitempty"`
	ID      string `json:"id,omitempty"`
}

func mapStatus(status string) sandbox.SandboxStatus {
	switch strings.ToLower(status) {
	case "running":
		return sandbox.StatusRunning
	case "stopped":
		return sandbox.StatusStopped
	case "creating":
		return sandbox.StatusCreating
	case "upgrading":
		return sandbox.StatusUpgrading
	case "error":
		return sandbox.StatusError
	default:
		return sandbox.StatusError
	}
}
