-- +goose Up
CREATE TABLE public.rag_sync_records (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    entity_id uuid NOT NULL,
    sync_type text NOT NULL,
    sync_status text NOT NULL,
    num_docs_synced bigint DEFAULT 0 NOT NULL,
    sync_start_time timestamp with time zone NOT NULL,
    sync_end_time timestamp with time zone
);

ALTER TABLE ONLY public.rag_sync_records
    ADD CONSTRAINT rag_sync_records_pkey PRIMARY KEY (id);

CREATE INDEX idx_rag_sync_records_org_id ON public.rag_sync_records USING btree (org_id);
