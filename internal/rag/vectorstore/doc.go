// Package vectorstore wraps Qdrant and exposes the abstract
// vector-store operations the rest of the RAG system depends on.
//
// Qdrant is Hivy's vector search backend; this package is the single
// integration point the rest of the RAG system depends on.
package vectorstore
