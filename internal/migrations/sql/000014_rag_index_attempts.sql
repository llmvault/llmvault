-- +goose Up
CREATE TABLE public.rag_index_attempts (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    rag_source_id uuid NOT NULL,
    embedding_model_id text,
    from_beginning boolean DEFAULT false NOT NULL,
    status text NOT NULL,
    new_docs_indexed bigint DEFAULT 0,
    total_docs_indexed bigint DEFAULT 0,
    docs_removed_from_index bigint DEFAULT 0,
    docs_estimated integer,
    error_msg text,
    full_exception_trace text,
    poll_range_start timestamp with time zone,
    poll_range_end timestamp with time zone,
    checkpoint_pointer text,
    celery_task_id text,
    cancellation_requested boolean DEFAULT false NOT NULL,
    total_batches bigint,
    completed_batches bigint DEFAULT 0 NOT NULL,
    total_failures_batch_level bigint DEFAULT 0 NOT NULL,
    total_chunks bigint DEFAULT 0 NOT NULL,
    last_progress_time timestamp with time zone,
    last_batches_completed_count bigint DEFAULT 0 NOT NULL,
    heartbeat_counter bigint DEFAULT 0 NOT NULL,
    last_heartbeat_value bigint DEFAULT 0 NOT NULL,
    last_heartbeat_time timestamp with time zone,
    time_created timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    time_started timestamp with time zone,
    time_updated timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);

ALTER TABLE ONLY public.rag_index_attempts
    ADD CONSTRAINT rag_index_attempts_pkey PRIMARY KEY (id);

CREATE INDEX idx_rag_index_attempts_org_id ON public.rag_index_attempts USING btree (org_id);

CREATE INDEX idx_rag_index_attempts_rag_source_id ON public.rag_index_attempts USING btree (rag_source_id);

CREATE INDEX idx_rag_index_attempts_status ON public.rag_index_attempts USING btree (status);

CREATE INDEX idx_rag_index_attempts_time_created ON public.rag_index_attempts USING btree (time_created);

-- +goose Down
DROP TABLE IF EXISTS public.rag_index_attempts CASCADE;
