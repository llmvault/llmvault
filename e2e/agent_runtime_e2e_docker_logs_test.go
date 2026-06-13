package e2e

import (
	"bufio"
	"context"
	"io"
	"os/exec"
	"sync"
	"testing"
	"time"
)

func streamDockerLogs(t *testing.T, trace *agentRuntimeE2ETrace, containerID string) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "docker", "logs", "-f", containerID)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		trace.Logf("runtime-log", "stdout pipe unavailable: %v", err)
		cancel()
		return func() {}
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		trace.Logf("runtime-log", "stderr pipe unavailable: %v", err)
		cancel()
		return func() {}
	}
	if err := cmd.Start(); err != nil {
		trace.Logf("runtime-log", "docker logs -f failed to start: %v", err)
		cancel()
		return func() {}
	}
	trace.Logf("runtime-log", "streaming docker logs for container=%s", containerID)

	var wg sync.WaitGroup
	wg.Add(2)
	go scanDockerLogStream(trace, &wg, "stdout", stdout)
	go scanDockerLogStream(trace, &wg, "stderr", stderr)

	return func() {
		cancel()
		done := make(chan struct{})
		go func() {
			_ = cmd.Wait()
			wg.Wait()
			close(done)
		}()
		select {
		case <-done:
			trace.Logf("runtime-log", "docker log stream stopped")
		case <-time.After(5 * time.Second):
			trace.Logf("runtime-log", "docker log stream stop timed out")
		}
	}
}

func scanDockerLogStream(trace *agentRuntimeE2ETrace, wg *sync.WaitGroup, stream string, reader io.Reader) {
	defer wg.Done()
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	for scanner.Scan() {
		trace.Logf("runtime-log", "%s %s", stream, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		trace.Logf("runtime-log", "%s scanner error: %v", stream, err)
	}
}
