// Package vectorstore wraps Qdrant and exposes the abstract
// vector-store operations the rest of the RAG system depends on.
//
// Ports backend/onyx/document_index/vespa/index.py — the analogous Vespa
// client — but swapped to Qdrant for Hivy's vector search backend.
package vectorstore
