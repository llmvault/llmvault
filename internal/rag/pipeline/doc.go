// Package pipeline orchestrates the end-to-end indexing pipeline: fetch,
// chunk, embed, write to Qdrant + Postgres.
//
// It composes the indexing stages into a single pipeline and runs a
// document batch through each stage in order.
package pipeline
