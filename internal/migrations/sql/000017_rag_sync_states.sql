-- +goose Up
CREATE TABLE public.rag_sync_states (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    rag_source_id uuid NOT NULL,
    status character varying(32) NOT NULL,
    in_repeated_error_state boolean DEFAULT false NOT NULL,
    last_successful_index_time timestamp with time zone,
    last_pruned timestamp with time zone,
    last_time_hierarchy_fetch timestamp with time zone,
    total_docs_indexed bigint DEFAULT 0 NOT NULL,
    indexing_trigger character varying(16),
    processing_mode character varying(16) DEFAULT 'REGULAR'::character varying NOT NULL,
    deletion_failure_message text,
    creator_id uuid,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY public.rag_sync_states
    ADD CONSTRAINT rag_sync_states_pkey PRIMARY KEY (id);

CREATE INDEX idx_rag_sync_state_last_pruned ON public.rag_sync_states USING btree (last_pruned);

CREATE INDEX idx_rag_sync_states_org_id ON public.rag_sync_states USING btree (org_id);

CREATE UNIQUE INDEX uq_rag_sync_state_rag_source_id ON public.rag_sync_states USING btree (rag_source_id);
