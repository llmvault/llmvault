// Package locks provides Redis-backed coordination primitives for the
// three-loop sync architecture — per-connection single-flight,
// fencing-token locks, and heartbeat-liveness checks.
//
// Every sync loop (ingest, perm-sync, prune) acquires a per-connection
// Redis lock for the duration of its run so a connection is never
// processed by two workers at once.
package locks
