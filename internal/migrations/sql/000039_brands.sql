-- +goose Up
CREATE TABLE brands (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    name text NOT NULL,
    slug text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    logos jsonb DEFAULT '{}'::jsonb NOT NULL,
    colors jsonb DEFAULT '{}'::jsonb NOT NULL,
    typography jsonb DEFAULT '{}'::jsonb NOT NULL,
    voice jsonb DEFAULT '{}'::jsonb NOT NULL,
    source jsonb DEFAULT '{"version":1,"origin":"manual"}'::jsonb NOT NULL,
    raw_import jsonb,
    archived_at timestamp with time zone,
    created_by uuid,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE brand_assets (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    brand_id uuid NOT NULL,
    kind text NOT NULL,
    role text DEFAULT ''::text NOT NULL,
    name text NOT NULL,
    key text NOT NULL,
    public_url text NOT NULL,
    content_type text NOT NULL,
    bytes bigint NOT NULL,
    width integer,
    height integer,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_by uuid,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    CONSTRAINT brand_assets_kind_check CHECK (kind IN ('logo', 'mark', 'icon', 'image', 'font', 'document', 'other')),
    CONSTRAINT brand_assets_bytes_check CHECK (bytes > 0),
    CONSTRAINT brand_assets_width_check CHECK (width IS NULL OR width > 0),
    CONSTRAINT brand_assets_height_check CHECK (height IS NULL OR height > 0)
);

ALTER TABLE brands ADD CONSTRAINT brands_pkey PRIMARY KEY (id);
ALTER TABLE brand_assets ADD CONSTRAINT brand_assets_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX idx_brands_org_slug_active ON brands USING btree (org_id, slug) WHERE archived_at IS NULL;
CREATE UNIQUE INDEX idx_brands_org_default_active ON brands USING btree (org_id) WHERE is_default AND archived_at IS NULL;
CREATE INDEX idx_brands_org_created ON brands USING btree (org_id, created_at DESC) WHERE archived_at IS NULL;
CREATE INDEX idx_brands_archived_at ON brands USING btree (archived_at);

CREATE UNIQUE INDEX idx_brand_assets_key ON brand_assets USING btree (key);
CREATE INDEX idx_brand_assets_org_brand ON brand_assets USING btree (org_id, brand_id);
CREATE INDEX idx_brand_assets_kind ON brand_assets USING btree (kind);

ALTER TABLE ONLY brands
    ADD CONSTRAINT fk_brands_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;
ALTER TABLE ONLY brands
    ADD CONSTRAINT fk_brands_created_by FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE ONLY brand_assets
    ADD CONSTRAINT fk_brand_assets_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;
ALTER TABLE ONLY brand_assets
    ADD CONSTRAINT fk_brand_assets_brand FOREIGN KEY (brand_id) REFERENCES brands(id) ON DELETE CASCADE;
ALTER TABLE ONLY brand_assets
    ADD CONSTRAINT fk_brand_assets_created_by FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;

-- +goose Down
DROP TABLE IF EXISTS brand_assets;
DROP TABLE IF EXISTS brands;
