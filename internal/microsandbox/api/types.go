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

func DefaultPreviewPorts() []int {
	return []int{
		3000, 3001, 3002, 3003, 3004,
		3333,
		4000, 4001,
		4173, 4200, 4321,
		5000, 5001,
		5173, 5174,
		6006,
		7000, 7070, 7080,
		8000, 8001,
		8080, 8081, 8082, 8088,
		8100, 8501, 8787,
		9000, 9001,
	}
}

type ErrorResponse struct {
	Error string `json:"error"`
}
