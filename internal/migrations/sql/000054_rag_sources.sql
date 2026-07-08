-- +goose Up
CREATE TABLE public.rag_sources (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    kind character varying(32) NOT NULL,
    name text NOT NULL,
    status character varying(32) NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    config jsonb DEFAULT '{}'::jsonb NOT NULL,
    connection_id uuid,
    indexing_start timestamp with time zone,
    last_successful_index_time timestamp with time zone,
    last_pruned timestamp with time zone,
    refresh_freq_seconds integer,
    prune_freq_seconds integer,
    total_docs_indexed bigint DEFAULT 0 NOT NULL,
    in_repeated_error_state boolean DEFAULT false NOT NULL,
    deletion_failure_message text,
    creator_id uuid,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY public.rag_sources
    ADD CONSTRAINT rag_sources_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.rag_sources
    ADD CONSTRAINT fk_rag_sources_connection FOREIGN KEY (connection_id) REFERENCES public.connections(id);
