package docker

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/errdefs"
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
		if errdefs.IsNotFound(err) {
			return "", sandbox.ErrSandboxNotFound
		}
		return "", fmt.Errorf("creating docker exec in %s: %w", externalID, err)
	}

	attached, err := d.cli.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{})
	if err != nil {
		if errdefs.IsNotFound(err) {
			return "", sandbox.ErrSandboxNotFound
		}
		return "", fmt.Errorf("attaching docker exec in %s: %w", externalID, err)
	}
	defer attached.Close()

	// stdcopy.StdCopy blocks reading the hijacked connection and does not honor
	// ctx on its own. Run it in a goroutine and select on ctx so a timeout
	// unblocks us; closing the connection forces the in-flight read to return.
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

// wrapWithTimeout wraps a command in `timeout <secs>` so the in-container
// process is actually killed when the deadline fires. Cancelling our ctx alone
// tears down the hijacked read but leaves the shell running inside the
// container; the SIGKILL fallback (-k) covers commands that ignore the initial
// SIGTERM. The timeout is rounded up so a sub-second deadline still arms
// `timeout` with at least 1s.
func wrapWithTimeout(command string, timeout time.Duration) string {
	secs := int(math.Ceil(timeout.Seconds()))
	if secs < 1 {
		secs = 1
	}
	return fmt.Sprintf("timeout -k 5s %ds /bin/sh -c %s", secs, shellQuote(command))
}

// shellQuote wraps s in single quotes for safe interpolation into a /bin/sh
// command line, escaping any embedded single quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
