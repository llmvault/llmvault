package runner

import (
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/usehivy/hivy/internal/microsandbox/api"
)

type cpuTimes struct {
	total uint64
	idle  uint64
}

type hostPressureSampler struct {
	mu       sync.Mutex
	previous cpuTimes
	hasPrev  bool
}

func newHostPressureSampler() *hostPressureSampler {
	return &hostPressureSampler{}
}

func (s *hostPressureSampler) Sample(startingOperations int) api.RunnerPressure {
	pressure := api.RunnerPressure{StartingOperations: startingOperations}
	current, ok := readCPUTimes("/proc/stat")
	s.mu.Lock()
	if ok && s.hasPrev && current.total > s.previous.total {
		totalDelta := current.total - s.previous.total
		idleDelta := current.idle - s.previous.idle
		pressure.CPUUtilization = 100 * (1 - float64(idleDelta)/float64(totalDelta))
	}
	if ok {
		s.previous = current
		s.hasPrev = true
	}
	s.mu.Unlock()
	pressure.Load1, pressure.RunnableProcesses = readLoadPressure("/proc/loadavg")
	return pressure
}

func readCPUTimes(path string) (cpuTimes, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return cpuTimes{}, false
	}
	line, _, _ := strings.Cut(string(raw), "\n")
	fields := strings.Fields(line)
	if len(fields) < 6 || fields[0] != "cpu" {
		return cpuTimes{}, false
	}
	values := make([]uint64, 0, len(fields)-1)
	for _, field := range fields[1:] {
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return cpuTimes{}, false
		}
		values = append(values, value)
	}
	var total uint64
	for i, value := range values {
		// guest and guest_nice are already included in user and nice.
		if i >= 8 {
			break
		}
		total += value
	}
	idle := values[3]
	if len(values) > 4 {
		idle += values[4]
	}
	return cpuTimes{total: total, idle: idle}, true
}

func readLoadPressure(path string) (float64, int) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, 0
	}
	fields := strings.Fields(string(raw))
	if len(fields) < 4 {
		return 0, 0
	}
	load1, _ := strconv.ParseFloat(fields[0], 64)
	runnableRaw, _, _ := strings.Cut(fields[3], "/")
	runnable, _ := strconv.Atoi(runnableRaw)
	return load1, runnable
}
