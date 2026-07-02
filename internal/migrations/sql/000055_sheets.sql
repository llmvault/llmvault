-- +goose Up
CREATE TABLE sheets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    slug text NOT NULL,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    icon text NOT NULL DEFAULT '',
    created_by_agent_id uuid REFERENCES agents(id) ON DELETE SET NULL,
    created_by_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    source_session_id uuid REFERENCES sessions(id) ON DELETE SET NULL,
    archived_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_sheets_org_slug_active
    ON sheets(org_id, slug)
    WHERE archived_at IS NULL;
CREATE INDEX idx_sheets_org_updated_active
    ON sheets(org_id, updated_at DESC)
    WHERE archived_at IS NULL;

CREATE TABLE sheet_pages (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    sheet_id uuid NOT NULL REFERENCES sheets(id) ON DELETE CASCADE,
    org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    name text NOT NULL,
    position double precision NOT NULL,
    display_field_id text,
    archived_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_sheet_pages_sheet_name_active
    ON sheet_pages(sheet_id, name)
    WHERE archived_at IS NULL;
CREATE INDEX idx_sheet_pages_org
    ON sheet_pages(org_id);

CREATE TABLE sheet_fields (
    id text PRIMARY KEY,
    page_id uuid NOT NULL REFERENCES sheet_pages(id) ON DELETE CASCADE,
    org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    name text NOT NULL,
    type text NOT NULL,
    options jsonb NOT NULL DEFAULT '{}'::jsonb,
    position double precision NOT NULL,
    archived_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_sheet_fields_page_name_active
    ON sheet_fields(page_id, name)
    WHERE archived_at IS NULL;
CREATE INDEX idx_sheet_fields_page
    ON sheet_fields(page_id);
CREATE INDEX idx_sheet_fields_org
    ON sheet_fields(org_id);

CREATE TABLE sheet_import_jobs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    page_id uuid NOT NULL REFERENCES sheet_pages(id) ON DELETE CASCADE,
    object_key text NOT NULL,
    status text NOT NULL DEFAULT 'pending',
    total_rows bigint NOT NULL DEFAULT 0,
    processed_rows bigint NOT NULL DEFAULT 0,
    error text NOT NULL DEFAULT '',
    options jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_by_agent_id uuid REFERENCES agents(id) ON DELETE SET NULL,
    created_by_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT sheet_import_jobs_status_check
        CHECK (status IN ('pending', 'running', 'completed', 'failed'))
);

CREATE INDEX idx_sheet_import_jobs_page
    ON sheet_import_jobs(page_id);
CREATE INDEX idx_sheet_import_jobs_org
    ON sheet_import_jobs(org_id);

CREATE TABLE sheet_rows (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    page_id uuid NOT NULL REFERENCES sheet_pages(id) ON DELETE CASCADE,
    org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    data jsonb NOT NULL DEFAULT '{}'::jsonb,
    position double precision NOT NULL,
    import_job_id uuid REFERENCES sheet_import_jobs(id) ON DELETE SET NULL,
    created_by_agent_id uuid REFERENCES agents(id) ON DELETE SET NULL,
    created_by_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    archived_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_sheet_rows_page_position_active
    ON sheet_rows(page_id, position)
    WHERE archived_at IS NULL;
CREATE INDEX idx_sheet_rows_page_created
    ON sheet_rows(page_id, created_at, id);
CREATE INDEX idx_sheet_rows_data_gin
    ON sheet_rows USING GIN (data jsonb_path_ops);
CREATE INDEX idx_sheet_rows_import_job
    ON sheet_rows(import_job_id)
    WHERE import_job_id IS NOT NULL;

CREATE TABLE sheet_views (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    page_id uuid NOT NULL REFERENCES sheet_pages(id) ON DELETE CASCADE,
    org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    name text NOT NULL,
    type text NOT NULL DEFAULT 'grid',
    config jsonb NOT NULL DEFAULT '{}'::jsonb,
    position double precision NOT NULL,
    archived_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_sheet_views_page_active
    ON sheet_views(page_id)
    WHERE archived_at IS NULL;
CREATE INDEX idx_sheet_views_org
    ON sheet_views(org_id);

CREATE TABLE sheet_operations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    page_id uuid NOT NULL REFERENCES sheet_pages(id) ON DELETE CASCADE,
    type text NOT NULL,
    row_count int NOT NULL DEFAULT 0,
    inverse jsonb NOT NULL DEFAULT '{}'::jsonb,
    actor_agent_id uuid REFERENCES agents(id) ON DELETE SET NULL,
    actor_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    source_session_id uuid REFERENCES sessions(id) ON DELETE SET NULL,
    reverted_at timestamptz,
    reverted_by_agent_id uuid REFERENCES agents(id) ON DELETE SET NULL,
    reverted_by_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT sheet_operations_type_check
        CHECK (type IN ('rows_insert', 'rows_update', 'rows_delete', 'csv_import', 'field_change'))
);

CREATE INDEX idx_sheet_operations_page_created
    ON sheet_operations(page_id, created_at DESC);
CREATE INDEX idx_sheet_operations_org
    ON sheet_operations(org_id);

-- +goose Down
DROP TABLE IF EXISTS sheet_operations;
DROP TABLE IF EXISTS sheet_views;
DROP TABLE IF EXISTS sheet_rows;
DROP TABLE IF EXISTS sheet_import_jobs;
DROP TABLE IF EXISTS sheet_fields;
DROP TABLE IF EXISTS sheet_pages;
DROP TABLE IF EXISTS sheets;
