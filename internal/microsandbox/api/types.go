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

type ErrorResponse struct {
	Error string `json:"error"`
}
