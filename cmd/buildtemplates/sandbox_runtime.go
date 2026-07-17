package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"

	daytona "github.com/daytona/clients/sdk-go/pkg/daytona"
	"github.com/daytona/clients/sdk-go/pkg/types"

	"github.com/usehivy/hivy/internal/model"
)

type runtimeVariant struct {
	imageRepo      string
	snapshotPrefix string
	displayName    string
	minimumDisk    int
}

const (
	daytonaMaxCPU      = 4
	daytonaMaxMemoryGB = 8
	daytonaMaxDiskGB   = 10
)

var (
	runtimeEntrypoint = []string{
		"/usr/local/bin/hivy-daytona-entrypoint",
	}
	runtimeDefaultVariant = runtimeVariant{
		imageRepo:      "ghcr.io/usehivy/hivy-sandboxes-runtime-daytona",
		snapshotPrefix: "hivy-sandboxes-runtime-daytona",
		displayName:    "usehivy/hivy-sandboxes-runtime-daytona",
		minimumDisk:    5,
	}
	runtimeDevelopersVariant = runtimeVariant{
		imageRepo:      "ghcr.io/usehivy/hivy-sandboxes-runtime-developers-daytona",
		snapshotPrefix: "hivy-sandboxes-runtime-developers-daytona",
		displayName:    "usehivy/hivy-sandboxes-runtime-developers-daytona",
		minimumDisk:    10,
	}
)

func runSandboxRuntime(ctx context.Context, args []string, variant runtimeVariant) {
	fs := flag.NewFlagSet("sandbox-runtime", flag.ExitOnError)
	version := fs.String("version", "", "Tag of "+variant.displayName+" already published to GHCR (required, e.g. v0.0.1)")
	size := fs.String("size", "all", "Snapshot sizes to register (micro, nano, small, medium, large, xlarge, all)")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if *version == "" {
		fmt.Fprintln(os.Stderr, "error: -version is required (e.g. v0.0.1)")
		os.Exit(1)
	}
	targetSizes, err := resolveSizes(*size)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if err := registerSandboxRuntimeSnapshots(ctx, *version, targetSizes, variant); err != nil {
		log.Fatalf("error: %v", err)
	}
	log.Println("Done.")
}

func registerSandboxRuntimeSnapshots(ctx context.Context, version string, targetSizes []string, variant runtimeVariant) error {
	cleanVersion := strings.TrimPrefix(version, "v")
	dashedVersion := strings.ReplaceAll(cleanVersion, ".", "-")
	imageRef := fmt.Sprintf("%s:%s-amd64", variant.imageRepo, strings.TrimSuffix(version, "-amd64"))
	if err := runCommand(ctx, nil, "docker", "pull", "--platform", "linux/amd64", imageRef); err != nil {
		return fmt.Errorf("pulling Daytona runtime image %q: %w", imageRef, err)
	}

	client, err := daytona.NewClientWithConfig(&types.DaytonaConfig{
		APIKey: os.Getenv("HIVY_DAYTONA_API_KEY"),
		APIUrl: os.Getenv("HIVY_DAYTONA_API_URL"),
		Target: os.Getenv("HIVY_DAYTONA_TARGET"),
	})
	if err != nil {
		return fmt.Errorf("creating daytona client: %w", err)
	}
	defer client.Close(ctx)

	for _, sizeName := range targetSizes {
		size, ok := sizes[sizeName]
		if !ok {
			return fmt.Errorf("unknown size: %s", sizeName)
		}
		name := sandboxRuntimeSnapshotName(variant.snapshotPrefix, dashedVersion, size.Name)
		params := sandboxRuntimeCreateParams(name, imageRef, size, variant.minimumDisk)
		resources := params.Resources
		if resources == nil {
			return fmt.Errorf("snapshot %q has no resource allocation", name)
		}
		if size.Name == "micro" {
			log.Printf("Daytona adjusts Hivy micro from %d CPU/%d GiB memory/%d GiB disk to %d CPU/%d GiB memory/%d GiB disk",
				size.CPU, size.Memory, size.Disk, resources.CPU, resources.Memory, resources.Disk)
		} else if size.CPU != resources.CPU || size.Memory != resources.Memory || size.Disk != resources.Disk {
			log.Printf("Daytona adjusts Hivy %s from %d CPU/%d GiB memory/%d GiB disk to %d CPU/%d GiB memory/%d GiB disk",
				size.Name, size.CPU, size.Memory, size.Disk, resources.CPU, resources.Memory, resources.Disk)
		}
		log.Printf("Registering Daytona snapshot %q from %s (cpu=%d, mem=%dGB, disk=%dGB)...",
			name, imageRef, resources.CPU, resources.Memory, resources.Disk)
		cliEnv := map[string]string{
			"DAYTONA_API_KEY": os.Getenv("HIVY_DAYTONA_API_KEY"),
			"DAYTONA_API_URL": os.Getenv("HIVY_DAYTONA_API_URL"),
			"DAYTONA_TARGET":  os.Getenv("HIVY_DAYTONA_TARGET"),
		}
		if err := runCommand(ctx, cliEnv,
			"daytona", "snapshot", "push", imageRef,
			"--name", name,
			"--entrypoint", runtimeEntrypoint[0],
			"--cpu", strconv.Itoa(resources.CPU),
			"--memory", strconv.Itoa(resources.Memory),
			"--disk", strconv.Itoa(resources.Disk),
		); err != nil {
			return fmt.Errorf("pushing snapshot %q: %w", name, err)
		}

		final, err := client.Snapshot.Get(ctx, name)
		if err != nil {
			return fmt.Errorf("re-fetching snapshot %q after build: %w", name, err)
		}
		if final.State != "active" {
			reason := ""
			if final.ErrorReason != nil {
				reason = *final.ErrorReason
			}
			return fmt.Errorf("snapshot %q ended in state %q: %s", name, final.State, reason)
		}
		log.Printf("✓ Snapshot %q ready (state=%s id=%s)", name, final.State, final.ID)
	}
	return nil
}

func runCommand(ctx context.Context, extraEnv map[string]string, name string, args ...string) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Env = make([]string, 0, len(os.Environ())+len(extraEnv))
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if _, overridden := extraEnv[key]; !overridden {
			command.Env = append(command.Env, item)
		}
	}
	for key, value := range extraEnv {
		command.Env = append(command.Env, key+"="+value)
	}
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}

func sandboxRuntimeCreateParams(name, imageRef string, size model.TemplateSize, minimumDisk int) *types.CreateSnapshotParams {
	cpu := min(size.CPU, daytonaMaxCPU)
	memory := min(size.Memory, daytonaMaxMemoryGB)
	if size.Name == "micro" && memory < 1 {
		memory = 1
	}
	disk := size.Disk
	if disk < minimumDisk {
		disk = minimumDisk
	}
	if disk > daytonaMaxDiskGB {
		disk = daytonaMaxDiskGB
	}
	return &types.CreateSnapshotParams{
		Name:       name,
		Image:      imageRef,
		Entrypoint: append([]string(nil), runtimeEntrypoint...),
		Resources: &types.Resources{
			CPU:    cpu,
			Memory: memory,
			Disk:   disk,
		},
	}
}

func sandboxRuntimeSnapshotName(prefix, dashedVersion, size string) string {
	return fmt.Sprintf("%s-%s-%s-v1", prefix, dashedVersion, size)
}
