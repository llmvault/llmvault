package metrics

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	asynqTasks = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hivy_asynq_tasks_total",
		Help: "Processed Asynq tasks by queue, type, and outcome.",
	}, []string{"queue", "task_type", "status"})
	asynqDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "hivy_asynq_task_duration_seconds",
		Help:    "Asynq task processing duration.",
		Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300, 900},
	}, []string{"queue", "task_type", "status"})
	asynqInFlight = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hivy_asynq_tasks_in_flight",
		Help: "Asynq tasks currently executing.",
	}, []string{"queue", "task_type"})
)

func init() {
	prometheus.MustRegister(asynqTasks, asynqDuration, asynqInFlight)
}

// AsynqMiddleware records task throughput, errors, duration, and concurrency.
func AsynqMiddleware() asynq.MiddlewareFunc {
	return func(next asynq.Handler) asynq.Handler {
		return asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) (err error) {
			queue, _ := asynq.GetQueueName(ctx)
			queue = boundedLabel(queue, "unknown")
			taskType := boundedLabel(task.Type(), "unknown")
			started := time.Now()
			asynqInFlight.WithLabelValues(queue, taskType).Inc()
			defer func() {
				asynqInFlight.WithLabelValues(queue, taskType).Dec()
				status := "success"
				if err != nil {
					status = "error"
				}
				if recovered := recover(); recovered != nil {
					status = "panic"
					asynqTasks.WithLabelValues(queue, taskType, status).Inc()
					asynqDuration.WithLabelValues(queue, taskType, status).Observe(time.Since(started).Seconds())
					panic(recovered)
				}
				asynqTasks.WithLabelValues(queue, taskType, status).Inc()
				asynqDuration.WithLabelValues(queue, taskType, status).Observe(time.Since(started).Seconds())
			}()
			err = next.ProcessTask(ctx, task)
			return err
		})
	}
}

type AsynqQueueCollector struct {
	inspector *asynq.Inspector
	queues    []string
	tasks     *prometheus.Desc
	latency   *prometheus.Desc
	memory    *prometheus.Desc
	processed *prometheus.Desc
	failed    *prometheus.Desc
	up        *prometheus.Desc
}

// NewAsynqQueueCollector creates a scrape-time collector backed by Asynq's
// read-only Inspector. Payloads are never read or exposed.
func NewAsynqQueueCollector(redisOpt asynq.RedisConnOpt, queues []string) *AsynqQueueCollector {
	return &AsynqQueueCollector{
		inspector: asynq.NewInspector(redisOpt),
		queues:    append([]string(nil), queues...),
		tasks: prometheus.NewDesc(
			"hivy_asynq_queue_tasks",
			"Current Asynq task count by queue and state.",
			[]string{"queue", "state"}, nil,
		),
		latency: prometheus.NewDesc(
			"hivy_asynq_queue_latency_seconds",
			"Age of the oldest pending task.",
			[]string{"queue"}, nil,
		),
		memory: prometheus.NewDesc(
			"hivy_asynq_queue_memory_bytes",
			"Approximate Redis memory used by the queue.",
			[]string{"queue"}, nil,
		),
		processed: prometheus.NewDesc(
			"hivy_asynq_queue_processed_total",
			"Total tasks processed by the queue.",
			[]string{"queue"}, nil,
		),
		failed: prometheus.NewDesc(
			"hivy_asynq_queue_failed_total",
			"Total tasks failed by the queue.",
			[]string{"queue"}, nil,
		),
		up: prometheus.NewDesc(
			"hivy_asynq_queue_collector_up",
			"Whether queue state was collected successfully.",
			[]string{"queue"}, nil,
		),
	}
}

func (c *AsynqQueueCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.tasks
	ch <- c.latency
	ch <- c.memory
	ch <- c.processed
	ch <- c.failed
	ch <- c.up
}

func (c *AsynqQueueCollector) Collect(ch chan<- prometheus.Metric) {
	for _, queue := range c.queues {
		info, err := c.inspector.GetQueueInfo(queue)
		if err != nil {
			ch <- prometheus.MustNewConstMetric(c.up, prometheus.GaugeValue, 0, queue)
			continue
		}
		ch <- prometheus.MustNewConstMetric(c.up, prometheus.GaugeValue, 1, queue)
		ch <- prometheus.MustNewConstMetric(c.latency, prometheus.GaugeValue, info.Latency.Seconds(), queue)
		ch <- prometheus.MustNewConstMetric(c.memory, prometheus.GaugeValue, float64(info.MemoryUsage), queue)
		ch <- prometheus.MustNewConstMetric(c.processed, prometheus.CounterValue, float64(info.ProcessedTotal), queue)
		ch <- prometheus.MustNewConstMetric(c.failed, prometheus.CounterValue, float64(info.FailedTotal), queue)
		states := map[string]int{
			"pending":     info.Pending,
			"active":      info.Active,
			"scheduled":   info.Scheduled,
			"retry":       info.Retry,
			"archived":    info.Archived,
			"completed":   info.Completed,
			"aggregating": info.Aggregating,
		}
		for state, count := range states {
			ch <- prometheus.MustNewConstMetric(c.tasks, prometheus.GaugeValue, float64(count), queue, state)
		}
	}
}

func (c *AsynqQueueCollector) Close() error {
	return c.inspector.Close()
}
