package control

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/microsandbox/api"
	"github.com/usehivy/hivy/internal/microsandbox/model"
)

const upgradeStatusRolledBack = "rolled_back"

func requestedUpgradePorts(requested []int, ports []model.SandboxPort) []int {
	if len(requested) > 0 {
		return requested
	}
	out := make([]int, 0, len(ports))
	for _, port := range ports {
		out = append(out, port.GuestPort)
	}
	sort.Ints(out)
	return out
}

func validatePortSetUnchanged(previewPorts []int, ports []model.SandboxPort) error {
	requested := map[int]struct{}{}
	for _, port := range previewPorts {
		requested[port] = struct{}{}
	}
	if len(requested) != len(ports) {
		return fmt.Errorf("preview_ports cannot change during upgrade")
	}
	for _, port := range ports {
		if _, ok := requested[port.GuestPort]; !ok {
			return fmt.Errorf("preview_ports cannot change during upgrade")
		}
	}
	return nil
}

func requestedUpgradeResources(req upgradeSandboxRequest, sb model.Sandbox) sandboxResources {
	res := sandboxResources{CPU: sb.CPU, MemoryMB: sb.MemoryMB, DiskGB: sb.DiskGB}
	if req.CPU > 0 {
		res.CPU = req.CPU
	}
	if req.MemoryMB > 0 {
		res.MemoryMB = req.MemoryMB
	}
	if req.DiskGB > 0 {
		res.DiskGB = req.DiskGB
	}
	return res
}

func (s *Server) adjustUpgradeResources(ctx context.Context, sb model.Sandbox, current, next sandboxResources) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var runner model.Runner
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&runner, "id = ?", sb.RunnerID).Error; err != nil {
			return err
		}
		increase := api.Size{
			CPU:      max(next.CPU-current.CPU, 0),
			MemoryMB: max(next.MemoryMB-current.MemoryMB, 0),
			DiskGB:   max(next.DiskGB-current.DiskGB, 0),
		}
		if increase.CPU > 0 || increase.MemoryMB > 0 || increase.DiskGB > 0 {
			if !runnerHasCapacity(runner, increase) {
				return fmt.Errorf("runner %s has insufficient capacity", runner.ID)
			}
			if err := reserveRunner(tx, &runner, increase); err != nil {
				return err
			}
		}
		decrease := api.Size{
			CPU:      max(current.CPU-next.CPU, 0),
			MemoryMB: max(current.MemoryMB-next.MemoryMB, 0),
			DiskGB:   max(current.DiskGB-next.DiskGB, 0),
		}
		if decrease.CPU > 0 || decrease.MemoryMB > 0 || decrease.DiskGB > 0 {
			return releaseRunner(tx, &runner, decrease)
		}
		return nil
	})
}

func (s *Server) markSandboxUpgrading(ctx context.Context, sb *model.Sandbox) error {
	if err := s.db.WithContext(ctx).Model(sb).Updates(map[string]any{
		"status":           model.SandboxStatusUpgrading,
		"error_message":    "",
		"route_generation": gorm.Expr("route_generation + 1"),
	}).Error; err != nil {
		return err
	}
	sb.Status = model.SandboxStatusUpgrading
	sb.ErrorMessage = ""
	sb.RouteGeneration++
	return nil
}

func (s *Server) markSandboxUpgradeFailed(ctx context.Context, sb *model.Sandbox, message string) error {
	return s.db.WithContext(ctx).Model(sb).Updates(map[string]any{
		"status":        model.SandboxStatusError,
		"error_message": message,
	}).Error
}

func (s *Server) markSandboxUpgradeRolledBack(ctx context.Context, sb *model.Sandbox, message string) error {
	return s.db.WithContext(ctx).Model(sb).Updates(map[string]any{
		"status":        model.SandboxStatusRunning,
		"error_message": message,
	}).Error
}

func (s *Server) finishSandboxUpgrade(ctx context.Context, sb *model.Sandbox, req upgradeSandboxRequest, resources sandboxResources, ports []model.SandboxPort, healthChecks map[int]sandboxHealthCheckRequest) error {
	metadata, _ := json.Marshal(req.Metadata)
	now := time.Now().UTC()
	sleepAfterAt := nextSleepAfter(*sb, now)
	name := upgradeSandboxName(req.Name, sb.Name)
	for i := range ports {
		if check, ok := healthChecks[ports[i].GuestPort]; ok {
			applyHealthCheckToPort(&ports[i], check)
		}
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, port := range ports {
			if err := tx.Save(&port).Error; err != nil {
				return err
			}
		}
		return tx.Model(sb).Updates(map[string]any{
			"name":             name,
			"image_ref":        req.ImageRef,
			"status":           model.SandboxStatusRunning,
			"cpu":              resources.CPU,
			"memory_mb":        resources.MemoryMB,
			"disk_gb":          resources.DiskGB,
			"metadata_json":    string(metadata),
			"error_message":    "",
			"stopped_at":       nil,
			"last_wake_at":     now,
			"last_wake_error":  "",
			"sleep_after_at":   sleepAfterAt,
			"route_generation": gorm.Expr("route_generation + 1"),
		}).Error
	}); err != nil {
		return err
	}
	sb.Name = name
	sb.ImageRef = req.ImageRef
	sb.Status = model.SandboxStatusRunning
	sb.CPU = resources.CPU
	sb.MemoryMB = resources.MemoryMB
	sb.DiskGB = resources.DiskGB
	sb.MetadataJSON = string(metadata)
	sb.ErrorMessage = ""
	sb.StoppedAt = nil
	sb.LastWakeAt = &now
	sb.LastWakeError = ""
	sb.SleepAfterAt = sleepAfterAt
	sb.RouteGeneration++
	return nil
}

func prepareUpgradeEnv(controlURL string, sb model.Sandbox, req *upgradeSandboxRequest) {
	if req.Env == nil {
		req.Env = map[string]string{}
	}
	req.Env["HIVY_MICROSANDBOX_ID"] = sb.ID
	if sb.ActivityToken != "" {
		req.Env["HIVY_MICROSANDBOX_ACTIVITY_TOKEN"] = sb.ActivityToken
	}
	if controlURL != "" {
		req.Env["HIVY_MICROSANDBOX_CONTROL_URL"] = controlURL
	}
}

func runnerUpgradeLabels(sb model.Sandbox, metadata map[string]any) map[string]string {
	labels := map[string]string{"org_id": sb.OrgID, "sandbox_id": sb.ID}
	for key, value := range metadata {
		if text, ok := value.(string); ok {
			labels[key] = text
		}
	}
	return labels
}

func runnerBindingsForPorts(ports []model.SandboxPort) []runnerPortBinding {
	out := make([]runnerPortBinding, 0, len(ports))
	for _, port := range ports {
		out = append(out, runnerPortBinding{GuestPort: port.GuestPort, HostPort: port.HostPort})
	}
	return out
}

func upgradeSandboxName(requested, fallback string) string {
	if requested != "" {
		return requested
	}
	if fallback != "" {
		return fallback
	}
	return "upgraded-" + uuid.NewString()[:8]
}
