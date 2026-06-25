// Package filestore stores raw connector payloads, chunked-doc artifacts,
// and indexing checkpoints in S3-compatible object storage.
//
// Ports backend/onyx/file_store/ — the FileStore protocol and its S3
// implementation. In Hivy we use an R2/MinIO bucket for raw payloads,
// checkpoints, and derived indexing artifacts.
package filestore
