# RAG Architecture — Decision Log

**Scope:** RAG backend decisions. Further decisions are
appended as they land. Every decision records its rationale
(or calls out a deliberate trade-off explicitly).

---

## 1. Vector store: Qdrant

**Decision.** Use Qdrant via `github.com/qdrant/go-client` for vector
storage and search. Qdrant runs as a normal service in Compose/dev and as
the production vector database; Go code talks to it through
`internal/rag/qdrant`.

**Why Qdrant over Weaviate / pgvector:**
- Mature Go client and operationally simple single-purpose vector service.
- Payload filtering supports the document ACL and source-scoped prune paths.
- Collection-level vector configuration gives explicit dimensionality checks.
- Qdrant is already part of the local and CI RAG service stack.

We deliberately avoid heavier search engines that require their own
operating discipline; Qdrant keeps the operational surface small.

---

## 2. One embedding model per org for the lifetime of their index

**Decision.** Each org picks one embedding model at first-index time and
that model is pinned for every chunk in their dataset. Switching models
requires `DELETE FROM rag_documents WHERE org_id = ?` plus a full
re-ingest.

**Why.** Mixing embeddings of different dimensionalities or geometries in
one vector space produces silently-wrong nearest-neighbor results. A live
background model-swap workflow (staging past/present/future models plus
multi-stage re-embed) would let orgs switch without a full re-ingest, but
we consciously drop that machinery — the added code surface is not worth
it for our traffic profile.

**Design note.** `RAGSearchSettings` is per-org and pinned rather than
global with live switchover.

---

## 3. Three-loop sync architecture: ingest, perm-sync, prune

**Decision.** Every `Connection` independently schedules three loops:

| Loop | Cadence (default) | Responsibility |
|---|---|---|
| Ingest (fetch + chunk + embed + write) | `refresh_freq` seconds | Fetch source docs, chunk, embed, and write vectors |
| Perm-sync (update ACLs on existing docs, no vector rewrite) | `perm_sync_freq` seconds | Refresh document ACLs without touching vectors |
| Prune (remove source-deleted docs from the index) | `prune_freq` seconds | Drop docs that no longer exist in the source |

Plus an external-group syncing loop running on its own cadence for
connectors with group-based ACLs.

Each loop holds a per-connection Redis lock for the duration of its run;
the scheduler skips a connection whose lock is held. A watchdog scans a
partial index on `last_progress_time` to recover stuck runs.

**Why three loops instead of one.** Permissions on a 100k-doc corpus
change more often than the docs themselves. Coupling the two loops means
every perm change triggers a re-embed. Qdrant payload updates make the
permission-sync path metadata-only.

---

## 4. ACL string format invariant

**Decision.** ACL tokens are written and read using a single canonical
string format. The prefix functions live in `internal/rag/acl/prefix.go`.

Tokens on a chunk look like:

    user_email:alice@example.com
    external_group:github_org_hivy_team_core
    PUBLIC

The `PUBLIC` literal is the `PublicDocPat` constant. A query ACL set is
built in Go, sorted deterministically, and passed into Qdrant's payload
filter.

**Invariant.** If the write-side prefix and read-side prefix drift by
even one character, the filter matches zero rows and queries silently
return empty. The acl package ships a pure-logic test that pins every
prefix function's output to its exact expected string.

---

## 5. R2 + MinIO back the filestore

**Decision.** One bucket per environment. Prefix-isolated:

    <bucket>/
      filestore/      # raw payloads, checkpoints, CSV exports
        <org-id>/
          raw/
          checkpoints/

**Why one bucket.** Simpler IAM, simpler lifecycle policies, and simpler
backup for object artifacts. Per-org isolation is logical (prefix) not
physical (bucket). Raw payloads and checkpoints are read/written to S3
under configurable prefixes.

**Dev/test.** MinIO runs in docker-compose with a pre-created
`hivy-rag-test` bucket.

---

## 6. Shared Postgres + `org_id` column vs schema-per-tenant

**Decision.** We keep Hivy's existing pattern — shared Postgres, `org_id
uuid` column on every row, row-level `WHERE org_id = ?` filtering
everywhere, rather than a Postgres schema per tenant. Rationale:

- Hivy already has this pattern (`internal/model/`).
- Qdrant payload filters carry `org_id`/source scoping for vector rows.
- Schema-per-tenant would force a parallel migration runner.

The cost — every query must carry `org_id` — is paid up-front in the
data-layer tests (FK cascade tests prove org-delete wipes every
descendant row).

---

## Appendix: decisions deferred to later phases

These are tracked here so future agents don't re-litigate them.

- Reranker: Qwen3-Reranker-0.6B via SiliconFlow. Pluggable.
- Chat / answer generation: out of scope entirely. Hivy already has
  its own chat subsystem; RAG only exposes `Search() -> []Chunk`.
- Connector auth: Nango only. No direct provider HTTP clients.
- UserGroup RBAC: not implemented. Hivy's `OrgMembership.Role`
  covers org-level access; RAG introduces document-level ACLs as a
  separate axis (see Decision 4).
