-- +goose Up
-- Remove per-document access control from RAG. Access control is now solely
-- the Qdrant filter (org_id + optional rag_source_id set for channel-scoped
-- sources), so the access_type columns, the perm-sync bookkeeping columns,
-- and the external identity/group tables are all dropped.

ALTER TABLE rag_sources DROP COLUMN IF EXISTS access_type;
ALTER TABLE rag_sources DROP COLUMN IF EXISTS perm_sync_freq_seconds;
ALTER TABLE rag_sources DROP COLUMN IF EXISTS last_time_perm_sync;

ALTER TABLE rag_sync_states DROP COLUMN IF EXISTS access_type;
ALTER TABLE rag_sync_states DROP COLUMN IF EXISTS auto_sync_options;
ALTER TABLE rag_sync_states DROP COLUMN IF EXISTS last_time_perm_sync;
ALTER TABLE rag_sync_states DROP COLUMN IF EXISTS last_time_external_group_sync;

-- Junction tables first, then the parent identity/group tables.
DROP TABLE IF EXISTS rag_user_external_user_groups;
DROP TABLE IF EXISTS rag_public_external_user_groups;
DROP TABLE IF EXISTS rag_external_identities;
DROP TABLE IF EXISTS rag_external_user_groups;

-- rag_external_identities_id_seq is OWNED BY the dropped table's id column and
-- is auto-dropped with it; the guard is defensive.
DROP SEQUENCE IF EXISTS rag_external_identities_id_seq;

-- +goose Down
-- Restore the dropped columns and tables. Restored columns carry defaults so
-- the re-add succeeds against existing rows (the originals were NOT NULL).

ALTER TABLE rag_sources ADD COLUMN IF NOT EXISTS access_type character varying(16) NOT NULL DEFAULT 'private';
ALTER TABLE rag_sources ADD COLUMN IF NOT EXISTS perm_sync_freq_seconds integer;
ALTER TABLE rag_sources ADD COLUMN IF NOT EXISTS last_time_perm_sync timestamp with time zone;

ALTER TABLE rag_sync_states ADD COLUMN IF NOT EXISTS access_type character varying(16) NOT NULL DEFAULT 'private';
ALTER TABLE rag_sync_states ADD COLUMN IF NOT EXISTS auto_sync_options jsonb;
ALTER TABLE rag_sync_states ADD COLUMN IF NOT EXISTS last_time_perm_sync timestamp with time zone;
ALTER TABLE rag_sync_states ADD COLUMN IF NOT EXISTS last_time_external_group_sync timestamp with time zone;

CREATE TABLE IF NOT EXISTS rag_external_identities (
    id bigint NOT NULL,
    org_id uuid NOT NULL,
    user_id uuid NOT NULL,
    rag_source_id uuid NOT NULL,
    provider text NOT NULL,
    external_user_id text NOT NULL,
    external_user_login text,
    external_user_emails text[],
    updated_at timestamp with time zone
);

CREATE SEQUENCE IF NOT EXISTS rag_external_identities_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE rag_external_identities_id_seq OWNED BY rag_external_identities.id;
ALTER TABLE ONLY rag_external_identities ALTER COLUMN id SET DEFAULT nextval('rag_external_identities_id_seq'::regclass);

ALTER TABLE ONLY rag_external_identities
    ADD CONSTRAINT rag_external_identities_pkey PRIMARY KEY (id);

CREATE INDEX idx_rag_external_identity_org ON rag_external_identities USING btree (org_id);
CREATE INDEX idx_rag_external_identity_source ON rag_external_identities USING btree (rag_source_id);
CREATE UNIQUE INDEX uq_rag_external_identity_provider_ext_id_org ON rag_external_identities USING btree (org_id, provider, external_user_id);
CREATE UNIQUE INDEX uq_rag_external_identity_user_source ON rag_external_identities USING btree (user_id, rag_source_id);

CREATE TABLE IF NOT EXISTS rag_external_user_groups (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    rag_source_id uuid NOT NULL,
    external_user_group_id text NOT NULL,
    display_name text NOT NULL,
    gives_anyone_access boolean DEFAULT false NOT NULL,
    member_emails text[],
    updated_at timestamp with time zone
);

ALTER TABLE ONLY rag_external_user_groups
    ADD CONSTRAINT rag_external_user_groups_pkey PRIMARY KEY (id);

CREATE INDEX idx_rag_external_user_groups_org_id ON rag_external_user_groups USING btree (org_id);
CREATE UNIQUE INDEX uq_rag_external_user_group_source_ext ON rag_external_user_groups USING btree (rag_source_id, external_user_group_id);

CREATE TABLE IF NOT EXISTS rag_public_external_user_groups (
    external_user_group_id text NOT NULL,
    rag_source_id uuid NOT NULL,
    stale boolean DEFAULT false NOT NULL
);

ALTER TABLE ONLY rag_public_external_user_groups
    ADD CONSTRAINT rag_public_external_user_groups_pkey PRIMARY KEY (external_user_group_id, rag_source_id);

CREATE TABLE IF NOT EXISTS rag_user_external_user_groups (
    user_id uuid NOT NULL,
    external_user_group_id text NOT NULL,
    rag_source_id uuid NOT NULL,
    stale boolean DEFAULT false NOT NULL
);

ALTER TABLE ONLY rag_user_external_user_groups
    ADD CONSTRAINT rag_user_external_user_groups_pkey PRIMARY KEY (user_id, external_user_group_id, rag_source_id);
