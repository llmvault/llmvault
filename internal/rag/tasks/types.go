package tasks

const (
	TypeRagScanIngestDue = "rag:scan_ingest_due"
	TypeRagScanPruneDue  = "rag:scan_prune_due"
	TypeRagWatchdog      = "rag:watchdog_stuck_attempts"

	TypeRagIngest = "rag:ingest"
	TypeRagPrune  = "rag:prune"
)

const (
	QueueRagWork = "rag_work"
)
