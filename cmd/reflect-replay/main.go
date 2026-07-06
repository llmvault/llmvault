// reflect-replay replays the session-reflection extraction prompt against
// stored session transcripts and prints the memories the pipeline would keep.
// It reuses the production transcript renderer, prompt builder, normalization,
// and cap from internal/tasks, so there is zero drift between the harness and
// the worker. It never writes memories or touches reflection state.
//
// Usage:
//
//	go run ./cmd/reflect-replay -session <session-uuid>
//	go run ./cmd/reflect-replay -limit 10
//	go run ./cmd/reflect-replay -session <id> -verbose        # include raw LLM response
//	go run ./cmd/reflect-replay -session <id> -dry-run        # print transcript+prompt, skip the LLM
//
// Environment:
//
//	HIVY_DATABASE_URL           Postgres DSN (required)
//	HIVY_REFLECTION_API_KEY     provider API key (or OPENROUTER_API_KEY); required unless -dry-run
//	HIVY_REFLECTION_PROVIDER    same knob production reads (default "openrouter")
//	HIVY_REFLECTION_MODEL       same knob production reads (default "gpt-5-mini")
//	HIVY_REFLECTION_TEMPERATURE same knob production reads (default 0.1)
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/db"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/tasks"
	"github.com/usehivy/hivy/internal/trigger/hivy"
)

func main() {
	sessionFlag := flag.String("session", "", "Session UUID to replay")
	limitFlag := flag.Int("limit", 0, "Replay the N most recently active sessions instead of -session")
	verbose := flag.Bool("verbose", false, "Print the raw LLM response for each session")
	dryRun := flag.Bool("dry-run", false, "Render the transcript and system prompt without calling the LLM")
	showTranscript := flag.Bool("transcript", false, "Print the rendered transcript for each session")
	apiKeyFlag := flag.String("api-key", "", "Provider API key; falls back to HIVY_REFLECTION_API_KEY, then OPENROUTER_API_KEY")
	envFile := flag.String("env-file", ".env", "Env file to load before connecting; set empty to disable")
	timeout := flag.Duration("timeout", 5*time.Minute, "Overall timeout")
	flag.Parse()

	if *sessionFlag == "" && *limitFlag <= 0 {
		fmt.Fprintln(os.Stderr, "error: pass -session <uuid> or -limit N")
		flag.Usage()
		os.Exit(1)
	}
	if err := loadEnvFile(*envFile); err != nil {
		log.Fatalf("load env file: %v", err)
	}
	databaseURL := strings.TrimSpace(os.Getenv("HIVY_DATABASE_URL"))
	if databaseURL == "" {
		log.Fatal("HIVY_DATABASE_URL is required; export it or provide an env file with -env-file")
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	database, err := db.New(ctx, databaseURL)
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}

	reg := registry.Global()
	providerID, modelID, temperature := tasks.ResolveReflectionModel(reg)
	fmt.Printf("reflection model: provider=%s model=%s temperature=%.2f\n", providerID, modelID, temperature)

	var client hivy.CompletionClient
	if !*dryRun {
		apiKey := firstNonEmpty(*apiKeyFlag, os.Getenv("HIVY_REFLECTION_API_KEY"), os.Getenv("OPENROUTER_API_KEY"))
		if apiKey == "" {
			log.Fatal("no API key: pass -api-key, set HIVY_REFLECTION_API_KEY or OPENROUTER_API_KEY, or use -dry-run")
		}
		baseURL := ""
		if provider, ok := reg.GetProvider(providerID); ok {
			baseURL = provider.API
		}
		client = hivy.NewCompletionClient(&model.Credential{ProviderID: providerID, BaseURL: baseURL}, apiKey)
	}

	sessionIDs, err := resolveSessionIDs(ctx, database, *sessionFlag, *limitFlag)
	if err != nil {
		log.Fatal(err)
	}
	failures := 0
	for _, sessionID := range sessionIDs {
		run, err := tasks.ReplaySessionReflection(ctx, database, client, modelID, sessionID)
		if run != nil {
			printRun(run, *verbose, *showTranscript)
		}
		if err != nil {
			failures++
			fmt.Fprintf(os.Stderr, "error: session %s: %v\n", sessionID, err)
		}
	}
	if failures > 0 {
		os.Exit(1)
	}
}

func resolveSessionIDs(ctx context.Context, database *gorm.DB, sessionFlag string, limit int) ([]uuid.UUID, error) {
	if sessionFlag != "" {
		sessionID, err := uuid.Parse(strings.TrimSpace(sessionFlag))
		if err != nil {
			return nil, fmt.Errorf("invalid -session %q: %w", sessionFlag, err)
		}
		return []uuid.UUID{sessionID}, nil
	}
	ids, err := tasks.RecentReflectableSessionIDs(ctx, database, limit)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no sessions with reflectable events found")
	}
	return ids, nil
}

func printRun(run *tasks.ReflectionReplayRun, verbose, showTranscript bool) {
	fmt.Printf("\n=== session %s", run.SessionID)
	if run.SessionName != "" {
		fmt.Printf(" (%s)", run.SessionName)
	}
	if run.ChannelName != "" {
		fmt.Printf(" [channel: %s]", run.ChannelName)
	}
	fmt.Printf(" — %d events ===\n", run.EventCount)
	if showTranscript {
		fmt.Println("--- transcript ---")
		fmt.Println(run.Transcript)
		fmt.Println("--- end transcript ---")
	}
	if run.Existing != "" {
		fmt.Println("existing memories:")
		fmt.Println(run.Existing)
	}
	if verbose && run.RawResponse != "" {
		fmt.Println("--- raw response ---")
		fmt.Println(strings.TrimSpace(run.RawResponse))
		fmt.Println("--- end raw response ---")
	}
	if run.Memories == nil && run.RawResponse == "" {
		fmt.Println("kept memories: (dry run — LLM not called)")
		return
	}
	fmt.Printf("kept memories: %d\n", len(run.Memories))
	for i, mem := range run.Memories {
		encoded, err := json.Marshal(mem)
		if err != nil {
			fmt.Printf("  %d. [%s] %s\n", i+1, mem.Kind, mem.Content)
			continue
		}
		fmt.Printf("  %d. %s\n", i+1, encoded)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func loadEnvFile(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	return scanner.Err()
}
