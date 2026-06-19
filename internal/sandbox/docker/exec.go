package docker

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"

	"github.com/usehivy/hivy/internal/sandbox"
)

const defaultExecTimeout = 2 * time.Minute

func (d *Driver) ExecuteCommand(ctx context.Context, externalID string, command string) (string, error) {
	return d.ExecuteCommandWithTimeout(ctx, externalID, command, defaultExecTimeout)
}

func (d *Driver) ExecuteCommandWithTimeout(ctx context.Context, externalID string, command string, timeout time.Duration) (string, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
		command = wrapWithTimeout(command, timeout)
	}

	execID, err := d.cli.ContainerExecCreate(ctx, externalID, container.ExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          []string{"/bin/sh", "-lc", command},
	})
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return "", sandbox.ErrSandboxNotFound
		}
		return "", fmt.Errorf("creating docker exec in %s: %w", externalID, err)
	}

	attached, err := d.cli.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{})
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return "", sandbox.ErrSandboxNotFound
		}
		return "", fmt.Errorf("attaching docker exec in %s: %w", externalID, err)
	}
	defer attached.Close()

	// stdcopy.StdCopy blocks on the hijacked connection and ignores ctx, so run it
	// in a goroutine and select on ctx; closing the connection unblocks the read.
	var output bytes.Buffer
	copyDone := make(chan error, 1)
	go func() {
		_, err := stdcopy.StdCopy(&output, &output, attached.Reader)
		copyDone <- err
	}()

	select {
	case <-ctx.Done():
		attached.Close()
		<-copyDone
		return output.String(), fmt.Errorf("docker exec in %s timed out: %w", externalID, ctx.Err())
	case err := <-copyDone:
		if err != nil {
			return output.String(), fmt.Errorf("reading docker exec output in %s: %w", externalID, err)
		}
	}

	inspect, err := d.cli.ContainerExecInspect(ctx, execID.ID)
	if err != nil {
		return output.String(), fmt.Errorf("inspecting docker exec in %s: %w", externalID, err)
	}
	if inspect.ExitCode != 0 {
		return output.String(), fmt.Errorf("docker exec in %s exited with code %d: %s", externalID, inspect.ExitCode, strings.TrimSpace(output.String()))
	}
	return output.String(), nil
}

// wrapWithTimeout wraps a command in `timeout <secs>` so the in-container process is killed at the
// deadline (cancelling our ctx only tears down the read, leaving the shell running).
func wrapWithTimeout(command string, timeout time.Duration) string {
	secs := int(math.Ceil(timeout.Seconds()))
	if secs < 1 {
		secs = 1
	}
	return fmt.Sprintf("timeout -k 5s %ds /bin/sh -c %s", secs, shellQuote(command))
}

// shellQuote single-quotes s for safe /bin/sh interpolation, escaping embedded quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
