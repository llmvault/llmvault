// Package notion implements a RAG connector that indexes Notion pages
// (and, optionally, database rows) reachable by a Nango-authorised
// integration.
//
// Two indexing modes are supported, selected by config:
//
//   - WORKSPACE mode (no root pages configured): every page the
//     integration can see is enumerated via the search API and indexed.
//     Incremental polling is honoured client-side because the search
//     API exposes only a last-edited sort, not a time-range filter.
//   - SUBTREE mode (root pages configured): the connector seeds a page
//     frontier from the configured roots and walks child pages/database
//     rows recursively, ignoring the poll window (full re-walk each run).
//
// Bearer credentials never touch this package — every call is proxied
// through Nango, which injects auth. The connector only adds the
// Notion-Version and Content-Type headers the API requires.
package notion
