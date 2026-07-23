package infrastructure_test

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type cronJobManifest struct {
	Spec struct {
		TimeZone                string `yaml:"timeZone"`
		StartingDeadlineSeconds int    `yaml:"startingDeadlineSeconds"`
		JobTemplate             struct {
			Spec struct {
				Template struct {
					Spec podSpec `yaml:"spec"`
				} `yaml:"template"`
			} `yaml:"spec"`
		} `yaml:"jobTemplate"`
	} `yaml:"spec"`
}

type podSpec struct {
	AutomountServiceAccountToken *bool       `yaml:"automountServiceAccountToken"`
	InitContainers               []container `yaml:"initContainers"`
	Containers                   []container `yaml:"containers"`
}

type container struct {
	Name    string   `yaml:"name"`
	Command []string `yaml:"command"`
	Env     []envVar `yaml:"env"`
}

type envVar struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test filename")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func readYAML[T any](t *testing.T, path string) T {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var value T
	if err := yaml.Unmarshal(data, &value); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return value
}

func envValue(values []envVar, name string) string {
	for _, value := range values {
		if value.Name == name {
			return value.Value
		}
	}
	return ""
}

func TestRedisBackupManifestCannotReorderValidationBeforeExport(t *testing.T) {
	root := repoRoot(t)
	base := readYAML[cronJobManifest](t,
		filepath.Join(root, "kubernetes/apps/data-backups/redis.yaml"))
	pod := base.Spec.JobTemplate.Spec.Template.Spec

	if base.Spec.TimeZone != "Etc/UTC" {
		t.Fatalf("timeZone = %q, want Etc/UTC", base.Spec.TimeZone)
	}
	if base.Spec.StartingDeadlineSeconds == 0 {
		t.Fatal("startingDeadlineSeconds must bound delayed backup starts")
	}
	if pod.AutomountServiceAccountToken == nil || *pod.AutomountServiceAccountToken {
		t.Fatal("backup Pod must not mount a Kubernetes service-account token")
	}
	if len(pod.InitContainers) != 1 || pod.InitContainers[0].Name != "backup-and-validate" {
		t.Fatalf("init containers = %#v, want one atomic backup-and-validate step", pod.InitContainers)
	}
	if len(pod.InitContainers[0].Command) != 1 ||
		pod.InitContainers[0].Command[0] != "/opt/hivy/redis-backup/backup.sh" {
		t.Fatalf("backup command = %#v", pod.InitContainers[0].Command)
	}
	if envValue(pod.InitContainers[0].Env, "REDIS_BACKUP_MODE") != "cluster" {
		t.Fatal("base backup mode must be cluster")
	}

	for _, environment := range []struct {
		name string
		mode string
	}{
		{name: "production", mode: ""},
		{name: "staging", mode: "standalone"},
	} {
		patch := readYAML[cronJobManifest](t, filepath.Join(
			root,
			"kubernetes/environments",
			environment.name,
			"redis-backup-patch.yaml",
		))
		patchPod := patch.Spec.JobTemplate.Spec.Template.Spec
		if len(patchPod.InitContainers) != 1 ||
			patchPod.InitContainers[0].Name != "backup-and-validate" {
			t.Fatalf("%s patch can introduce init-container reordering: %#v",
				environment.name, patchPod.InitContainers)
		}
		if environment.mode != "" &&
			envValue(patchPod.InitContainers[0].Env, "REDIS_BACKUP_MODE") != environment.mode {
			t.Fatalf("%s backup mode is not %q", environment.name, environment.mode)
		}
	}
}

func TestRedisBackupScriptsContainBoundedLoadAndUploadVerification(t *testing.T) {
	root := repoRoot(t)
	backupPath := filepath.Join(
		root, "kubernetes/apps/data-backups/scripts/redis-backup.sh")
	uploadPath := filepath.Join(
		root, "kubernetes/apps/data-backups/scripts/redis-upload.sh")

	for _, path := range []string{backupPath, uploadPath} {
		output, err := exec.Command("sh", "-n", path).CombinedOutput()
		if err != nil {
			t.Fatalf("shell syntax %s: %v\n%s", path, err, output)
		}
	}

	backup, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	backupText := string(backup)
	for _, required := range []string{
		"redis-check-rdb",
		`while [ "${attempt}" -lt 60 ]`,
		"cluster_slots_assigned:16384",
		"cmp -s",
		"manifest.sha256",
	} {
		if !strings.Contains(backupText, required) {
			t.Fatalf("backup script is missing %q", required)
		}
	}

	upload, err := os.ReadFile(uploadPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(upload), "--query ContentLength") {
		t.Fatal("upload script must verify every remote object size")
	}
}

func TestRedisUploadVerifiesEveryExpectedClusterObject(t *testing.T) {
	root := repoRoot(t)
	tempDir := t.TempDir()
	backupTime := "20260723T030000Z"
	backupDir := filepath.Join(tempDir, backupTime)
	if err := os.Mkdir(backupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(tempDir, "latest-backup-time"),
		[]byte(backupTime+"\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	expected := []string{
		"leader-0.rdb",
		"leader-1.rdb",
		"leader-2.rdb",
		"cluster-nodes.txt",
		"cluster-info.txt",
		"manifest.sha256",
	}
	for index, name := range expected {
		content := strings.Repeat(name, index+1)
		if err := os.WriteFile(filepath.Join(backupDir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	binDir := filepath.Join(tempDir, "bin")
	if err := os.Mkdir(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	awsLog := filepath.Join(tempDir, "aws.log")
	fakeAWS := `#!/bin/sh
set -eu
if [ "$3" = "s3" ] && [ "$4" = "cp" ]; then
  exit 0
fi
key=
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--key" ]; then
    key="$2"
    break
  fi
  shift
done
filename="${key##*/}"
printf '%s\n' "${filename}" >>"${FAKE_AWS_LOG}"
wc -c <"${FAKE_BACKUP_DIR}/${filename}" | tr -d ' '
`
	fakeAWSPath := filepath.Join(binDir, "aws")
	// The owner-executable bit is required because PATH invokes this isolated
	// test double as the aws process.
	if err := os.WriteFile(fakeAWSPath, []byte(fakeAWS), 0o500); err != nil { //nolint:gosec
		t.Fatal(err)
	}

	uploadPath := filepath.Join(
		root, "kubernetes/apps/data-backups/scripts/redis-upload.sh")
	command := exec.Command("sh", uploadPath)
	command.Env = append(os.Environ(),
		"PATH="+binDir+":"+os.Getenv("PATH"),
		"REDIS_BACKUP_ROOT="+tempDir,
		"REDIS_BACKUP_MODE=cluster",
		"S3_ENDPOINT=https://s3.example.test",
		"S3_BUCKET=test-backups",
		"S3_PREFIX=redis",
		"FAKE_AWS_LOG="+awsLog,
		"FAKE_BACKUP_DIR="+backupDir,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("upload script: %v\n%s", err, output)
	}

	logged, err := os.ReadFile(awsLog)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Fields(string(logged))
	if strings.Join(got, ",") != strings.Join(expected, ",") {
		t.Fatalf("verified objects = %v, want %v", got, expected)
	}
}

func TestK3sPodResolverHasAtMostThreeRoutableNameservers(t *testing.T) {
	root := repoRoot(t)
	vars := readYAML[struct {
		ResolverPath string   `yaml:"k3s_resolv_conf_path"`
		Nameservers  []string `yaml:"k3s_dns_nameservers"`
	}](t, filepath.Join(root, "ansible/inventory/group_vars/k3s_servers.yml"))

	if vars.ResolverPath != "/etc/rancher/k3s/resolv.conf" {
		t.Fatalf("resolver path = %q", vars.ResolverPath)
	}
	if len(vars.Nameservers) == 0 || len(vars.Nameservers) > 3 {
		t.Fatalf("nameserver count = %d, want 1..3", len(vars.Nameservers))
	}
	for _, value := range vars.Nameservers {
		ip := net.ParseIP(value)
		if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			t.Fatalf("nameserver %q is not a routable IP address", value)
		}
	}

	configTemplate, err := os.ReadFile(filepath.Join(
		root, "ansible/roles/k3s-server/templates/config.yaml.j2"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(configTemplate),
		"resolv-conf: {{ k3s_resolv_conf_path }}") {
		t.Fatal("K3s configuration does not use the managed resolver file")
	}

	installPlaybook := readYAML[[]struct {
		Hosts  string `yaml:"hosts"`
		Serial int    `yaml:"serial"`
	}](t, filepath.Join(root, "ansible/playbooks/k3s/install.yml"))
	if len(installPlaybook) != 1 ||
		installPlaybook[0].Hosts != "k3s_servers" ||
		installPlaybook[0].Serial != 1 {
		t.Fatal("K3s reconciliation must roll through one server at a time")
	}
}
