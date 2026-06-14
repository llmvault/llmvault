package runner

import (
	"context"
	"fmt"
	"io"
	"time"

	microsandbox "github.com/superradcompany/microsandbox/sdk/go"
)

func (m *MicrosandboxBackend) Exec(ctx context.Context, sandboxID, command string, timeoutSeconds int) (*ExecResponse, error) {
	handle, err := microsandbox.GetSandbox(ctx, sandboxID)
	if err != nil {
		return nil, err
	}
	sb, err := handle.Connect(ctx)
	if err != nil {
		return nil, err
	}
	defer sb.Close()
	opts := []microsandbox.ExecOption{}
	if timeoutSeconds > 0 {
		opts = append(opts, microsandbox.WithExecTimeout(time.Duration(timeoutSeconds)*time.Second))
	}
	out, err := sb.Shell(ctx, command, opts...)
	if err != nil {
		return nil, err
	}
	return &ExecResponse{Stdout: out.Stdout(), Stderr: out.Stderr(), ExitCode: out.ExitCode()}, nil
}

func (m *MicrosandboxBackend) Logs(ctx context.Context, sandboxID string, w io.Writer) error {
	resp, err := m.Exec(ctx, sandboxID, "tail -200 /tmp/*.log 2>/dev/null || true", 10)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, resp.Stdout+resp.Stderr)
	return err
}

func (m *MicrosandboxBackend) Proxy(context.Context, string, int, io.Writer, io.Reader) error {
	return fmt.Errorf("direct proxy is handled by ProxyURL")
}

func (m *MicrosandboxBackend) ProxyURL(_ context.Context, sandboxID string, guestPort int) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ports := m.ports[sandboxID]
	hostPort := ports[guestPort]
	if hostPort == 0 {
		return "", fmt.Errorf("port %d is not published for sandbox %s", guestPort, sandboxID)
	}
	return fmt.Sprintf("http://127.0.0.1:%d", hostPort), nil
}
