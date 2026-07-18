// Command verify-devbox creates a sandbox from a Daytona runtime snapshot and
// checks startup, config delivery, preview access, command execution, and the
// developer image's Docker daemon.
//
// Usage:
//
//	go run ./cmd/verify-devbox -snapshot hivy-sandboxes-runtime-daytona-7-2-1-small-v1
//	go run ./cmd/verify-devbox -snapshot hivy-sandboxes-runtime-developers-daytona-7-2-1-small-v1
//	go run ./cmd/verify-devbox -cleanup <sandbox-id>
//
// Requires HIVY_DAYTONA_API_KEY, HIVY_DAYTONA_API_URL, and HIVY_DAYTONA_TARGET in
// the environment (load .env via the verify-devbox make target).
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	daytona "github.com/daytona/clients/sdk-go/pkg/daytona"
	"github.com/daytona/clients/sdk-go/pkg/types"

	"github.com/usehivy/hivy/internal/agentruntime"
)

const (
	runtimePort          = 7080
	runtimeHealthPath    = "/healthz"
	runtimeSecretEnv     = "HIVY_RUNTIME_SECRET" // #nosec G101 -- environment variable name, not a credential
	daytonaUser          = "daytona"
	daytonaHome          = "/home/daytona"
	daytonaWorkspaceRoot = daytonaHome
	daytonaDBPath        = daytonaHome + "/.hivy/runtime/hivy-sandboxes-runtime.db"
	signedPreviewTTL     = 24 * 60 * 60
)

func mustEnv(name string) string {
	value := os.Getenv(name)
	if value == "" {
		log.Fatalf("error: %s is required in the environment", name)
	}
	return value
}

func newClient(ctx context.Context) (*daytona.Client, error) {
	return daytona.NewClientWithConfig(&types.DaytonaConfig{
		APIKey: mustEnv("HIVY_DAYTONA_API_KEY"),
		APIUrl: mustEnv("HIVY_DAYTONA_API_URL"),
		Target: os.Getenv("HIVY_DAYTONA_TARGET"),
	})
}

func main() {
	snapshot := flag.String("snapshot", "", "Daytona runtime snapshot name to verify")
	developer := flag.Bool("developer", false, "Require Docker and run a Docker Compose workload (auto-detected for developer snapshot names)")
	keep := flag.Bool("keep", false, "Keep the sandbox after verification (for manual debugging)")
	cleanup := flag.String("cleanup", "", "Delete a sandbox by ID and exit (no verification)")
	flag.Parse()

	if *cleanup != "" {
		if err := runCleanup(*cleanup); err != nil {
			log.Fatalf("cleanup failed: %v", err)
		}
		return
	}

	if *snapshot == "" {
		fmt.Fprintln(os.Stderr, "error: -snapshot is required (or pass -cleanup <sandbox-id>)")
		flag.Usage()
		os.Exit(1)
	}

	if err := runVerify(*snapshot, *developer || strings.Contains(*snapshot, "developers"), *keep); err != nil {
		log.Fatalf("verification failed: %v", err)
	}
}

func runCleanup(sandboxID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client, err := newClient(ctx)
	if err != nil {
		return fmt.Errorf("creating daytona client: %w", err)
	}
	defer client.Close(ctx)

	log.Printf("Looking up sandbox %s...", sandboxID)
	sandbox, err := client.Get(ctx, sandboxID)
	if err != nil {
		return fmt.Errorf("getting sandbox: %w", err)
	}
	log.Printf("Found sandbox: id=%s name=%s state=%s", sandbox.ID, sandbox.Name, sandbox.State)

	log.Printf("Deleting sandbox %s...", sandbox.ID)
	if err := sandbox.Delete(ctx); err != nil {
		return fmt.Errorf("deleting sandbox: %w", err)
	}
	log.Printf("Sandbox deleted.")
	return nil
}

func runVerify(snapshot string, developer, keep bool) (retErr error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	client, err := newClient(ctx)
	if err != nil {
		return fmt.Errorf("creating daytona client: %w", err)
	}
	defer client.Close(ctx)

	runtimeSecret, err := generateRuntimeSecret()
	if err != nil {
		return fmt.Errorf("generating runtime secret: %w", err)
	}

	log.Printf("Creating sandbox from snapshot %q...", snapshot)
	sandbox, err := client.Create(ctx, sandboxParams(snapshot, runtimeSecret))
	if err != nil {
		return fmt.Errorf("creating sandbox: %w", err)
	}
	log.Printf("Sandbox created: id=%s name=%s state=%s", sandbox.ID, sandbox.Name, sandbox.State)

	if !keep {
		defer func() {
			log.Printf("Deleting sandbox %s...", sandbox.ID)
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cleanupCancel()
			if err := sandbox.Delete(cleanupCtx); err != nil {
				log.Printf("warning: failed to delete sandbox: %v", err)
			} else {
				log.Printf("Sandbox deleted.")
			}
		}()
	}

	log.Printf("Waiting for sandbox to start...")
	if err := sandbox.WaitForStart(ctx, 5*time.Minute); err != nil {
		return fmt.Errorf("waiting for sandbox start: %w", err)
	}
	log.Printf("Sandbox is running.")

	if err := verifySandboxProcess(ctx, sandbox, developer); err != nil {
		return err
	}

	log.Printf("Requesting a private signed Runtime preview URL...")
	preview, err := sandbox.GetSignedPreviewLink(ctx, runtimePort, signedPreviewTTL)
	if err != nil {
		return fmt.Errorf("getting signed Runtime preview link: %w", err)
	}
	runtimeURL := strings.TrimRight(preview.URL, "/")
	runtimeClient := agentruntime.NewClient(runtimeURL, runtimeSecret)

	if err := waitForRuntimeHealth(ctx, runtimeClient); err != nil {
		return err
	}
	if err := verifyBrowserPreviewCORS(ctx, runtimeURL+runtimeHealthPath); err != nil {
		return fmt.Errorf("checking browser preview CORS: %w", err)
	}
	log.Printf("Browser preview bypass and CORS check passed.")
	definition, err := runtimeClient.GetConfig(ctx)
	if err != nil {
		return fmt.Errorf("getting bootstrap Runtime config: %w", err)
	}
	if _, err := runtimeClient.PutRuntimeConfig(ctx, agentruntime.ConfigUpdateRequest{
		RuntimeSecret: runtimeSecret,
		RuntimeEnv: map[string]string{
			"HOME":                             daytonaHome,
			"HIVY_SANDBOX_DATA_ROOT":           daytonaHome + "/.hivy",
			agentruntime.AgentEnvDBPath:        daytonaDBPath,
			agentruntime.AgentEnvWorkspaceRoot: daytonaWorkspaceRoot,
		},
		Definition: definition,
		Workspace:  &agentruntime.WorkspaceConfig{Repos: []agentruntime.WorkspaceRepoConfig{}},
	}); err != nil {
		return fmt.Errorf("pushing Runtime config: %w", err)
	}
	if err := runtimeClient.Readyz(ctx); err != nil {
		return fmt.Errorf("checking Runtime readyz after config push: %w", err)
	}
	log.Printf("Runtime accepted config and reports ready.")
	if err := verifyBrowserSessionStreamCORS(ctx, runtimeURL, runtimeSecret); err != nil {
		return fmt.Errorf("checking authenticated browser session stream CORS: %w", err)
	}
	log.Printf("Authenticated browser session stream CORS check passed.")

	type portCheck struct {
		name     string
		port     int
		path     string
		required bool // false = diagnostic only, doesn't fail the run
	}
	checks := []portCheck{
		{name: "Runtime /healthz", port: runtimePort, path: runtimeHealthPath, required: true},
		{name: "sentinel (definitely unbound)", port: 9999, path: "/", required: false},
	}

	failed := 0
	for _, item := range checks {
		log.Printf("\n[%s] requesting preview link for port %d...", item.name, item.port)
		preview, err := sandbox.GetSignedPreviewLink(ctx, item.port, signedPreviewTTL)
		if err != nil {
			log.Printf("    ERROR getting preview link: %v", err)
			if item.required {
				failed++
			}
			continue
		}
		log.Printf("    signed private preview issued")

		fullURL := strings.TrimRight(preview.URL, "/") + item.path

		// Retry with backoff — services may take a few seconds to bind.
		var lastErr error
		var lastStatus int
		var lastBody string
		ok := false
		startedAt := time.Now()
		attempts := 1
		if item.required {
			attempts = 15
		}
		for attempt := 1; attempt <= attempts; attempt++ {
			status, body, err := httpGet(ctx, fullURL, "")
			lastErr = err
			lastStatus = status
			lastBody = body
			if err == nil && status >= 200 && status < 300 {
				log.Printf("    OK (HTTP %d, attempt %d, %s) -- body: %s",
					status, attempt, time.Since(startedAt).Round(time.Millisecond), truncate(body, 200))
				ok = true
				break
			}
			if attempts > 1 {
				time.Sleep(2 * time.Second)
			}
		}
		if !ok {
			label := "FAIL"
			if !item.required {
				label = "DIAGNOSTIC"
			}
			if lastErr != nil {
				log.Printf("    %s: %v", label, lastErr)
			} else {
				log.Printf("    %s: HTTP %d (after %s) -- body: %q",
					label, lastStatus, time.Since(startedAt).Round(time.Millisecond), truncate(lastBody, 400))
			}
			if item.required {
				failed++
			}
		}
	}

	log.Println()
	if failed > 0 {
		return fmt.Errorf("%d check(s) failed out of %d", failed, len(checks))
	}
	log.Printf("All %d checks passed for snapshot %q.", len(checks), snapshot)
	return nil
}

func generateRuntimeSecret() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
