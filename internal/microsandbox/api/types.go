package api

type Size struct {
	CPU      int `json:"cpu"`
	MemoryMB int `json:"memory_mb"`
	DiskGB   int `json:"disk_gb"`
}

var Sizes = map[string]Size{
	"small":  {CPU: 1, MemoryMB: 2048, DiskGB: 40},
	"medium": {CPU: 2, MemoryMB: 4096, DiskGB: 60},
	"large":  {CPU: 4, MemoryMB: 8192, DiskGB: 100},
	"xlarge": {CPU: 8, MemoryMB: 16384, DiskGB: 160},
}

const (
	DefaultSize                      = "small"
	DefaultPreviewHostPortRangeStart = 30000
	DefaultPreviewHostPortRangeEnd   = 60999
)

func DefaultPreviewPorts() []int {
	return []int{3000, 5173, 7080, 8000, 8080}
}

func PreviewPortCapacity(start, end, portsPerSandbox int) int {
	if start <= 0 || end < start || portsPerSandbox <= 0 {
		return 0
	}
	return ((end - start) + 1) / portsPerSandbox
}

type ErrorResponse struct {
	Error string `json:"error"`
}
