package control

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/microsandbox/model"
)

type fleetCollector struct {
	db              *gorm.DB
	up              *prometheus.Desc
	runnerStatus    *prometheus.Desc
	runnerCapacity  *prometheus.Desc
	runnerReserved  *prometheus.Desc
	runnerPressure  *prometheus.Desc
	runnerHeartbeat *prometheus.Desc
	sandboxes       *prometheus.Desc
}

func newFleetCollector(db *gorm.DB) *fleetCollector {
	return &fleetCollector{
		db: db,
		up: prometheus.NewDesc(
			"hivy_microsandbox_collector_up",
			"Whether microsandbox fleet state was collected successfully.",
			nil, nil,
		),
		runnerStatus: prometheus.NewDesc(
			"hivy_microsandbox_runner_status",
			"Runner status as a one-hot gauge.",
			[]string{"runner_id", "status"}, nil,
		),
		runnerCapacity: prometheus.NewDesc(
			"hivy_microsandbox_runner_capacity",
			"Configured runner capacity by resource.",
			[]string{"runner_id", "resource"}, nil,
		),
		runnerReserved: prometheus.NewDesc(
			"hivy_microsandbox_runner_reserved",
			"Reserved runner capacity by resource.",
			[]string{"runner_id", "resource"}, nil,
		),
		runnerPressure: prometheus.NewDesc(
			"hivy_microsandbox_runner_pressure",
			"Runner pressure signal.",
			[]string{"runner_id", "signal"}, nil,
		),
		runnerHeartbeat: prometheus.NewDesc(
			"hivy_microsandbox_runner_heartbeat_age_seconds",
			"Seconds since the runner's last heartbeat.",
			[]string{"runner_id"}, nil,
		),
		sandboxes: prometheus.NewDesc(
			"hivy_microsandbox_sandboxes",
			"Current microsandbox count by runner and lifecycle state.",
			[]string{"runner_id", "status"}, nil,
		),
	}
}

func (c *fleetCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.up
	ch <- c.runnerStatus
	ch <- c.runnerCapacity
	ch <- c.runnerReserved
	ch <- c.runnerPressure
	ch <- c.runnerHeartbeat
	ch <- c.sandboxes
}

func (c *fleetCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var runners []model.Runner
	if err := c.db.WithContext(ctx).Find(&runners).Error; err != nil {
		ch <- prometheus.MustNewConstMetric(c.up, prometheus.GaugeValue, 0)
		return
	}

	type sandboxCount struct {
		RunnerID string
		Status   string
		Count    int
	}
	var counts []sandboxCount
	if err := c.db.WithContext(ctx).
		Model(&model.Sandbox{}).
		Select("runner_id, status, count(*) AS count").
		Group("runner_id, status").
		Scan(&counts).Error; err != nil {
		ch <- prometheus.MustNewConstMetric(c.up, prometheus.GaugeValue, 0)
		return
	}

	ch <- prometheus.MustNewConstMetric(c.up, prometheus.GaugeValue, 1)
	now := time.Now()
	for _, runner := range runners {
		emitRunnerMetrics(ch, c, runner, now)
	}
	for _, count := range counts {
		ch <- prometheus.MustNewConstMetric(
			c.sandboxes,
			prometheus.GaugeValue,
			float64(count.Count),
			count.RunnerID,
			count.Status,
		)
	}
}

func emitRunnerMetrics(ch chan<- prometheus.Metric, c *fleetCollector, runner model.Runner, now time.Time) {
	statuses := []string{model.RunnerStatusHealthy, model.RunnerStatusUnhealthy}
	for _, status := range statuses {
		value := 0.0
		if runner.Status == status {
			value = 1
		}
		ch <- prometheus.MustNewConstMetric(c.runnerStatus, prometheus.GaugeValue, value, runner.ID, status)
	}

	resources := []struct {
		name     string
		capacity int
		reserved int
	}{
		{name: "cpu", capacity: runner.TotalCPU, reserved: runner.ReservedCPU},
		{name: "memory_mb", capacity: runner.TotalMemoryMB, reserved: runner.ReservedMemoryMB},
		{name: "disk_gb", capacity: runner.TotalDiskGB, reserved: runner.ReservedDiskGB},
	}
	for _, resource := range resources {
		ch <- prometheus.MustNewConstMetric(c.runnerCapacity, prometheus.GaugeValue, float64(resource.capacity), runner.ID, resource.name)
		ch <- prometheus.MustNewConstMetric(c.runnerReserved, prometheus.GaugeValue, float64(resource.reserved), runner.ID, resource.name)
	}

	pressure := map[string]float64{
		"cpu_utilization":     runner.CPUUtilization,
		"load_1m":             runner.Load1,
		"runnable_processes":  float64(runner.RunnableProcesses),
		"starting_operations": float64(runner.StartingOperations),
		"running_sandboxes":   float64(runner.ReportedRunningSandboxes),
	}
	for signal, value := range pressure {
		ch <- prometheus.MustNewConstMetric(c.runnerPressure, prometheus.GaugeValue, value, runner.ID, signal)
	}
	if runner.LastHeartbeatAt != nil {
		ch <- prometheus.MustNewConstMetric(c.runnerHeartbeat, prometheus.GaugeValue, now.Sub(*runner.LastHeartbeatAt).Seconds(), runner.ID)
	}
}
