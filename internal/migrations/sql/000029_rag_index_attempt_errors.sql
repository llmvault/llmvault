-- +goose Up
CREATE TABLE public.rag_index_attempt_errors (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    index_attempt_id uuid NOT NULL,
    rag_source_id uuid NOT NULL,
    document_id text,
    document_link text,
    entity_id text,
    failed_time_range_start timestamp with time zone,
    failed_time_range_end timestamp with time zone,
    failure_message text NOT NULL,
    is_resolved boolean DEFAULT false NOT NULL,
    error_type text,
    time_created timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);

ALTER TABLE ONLY public.rag_index_attempt_errors
    ADD CONSTRAINT rag_index_attempt_errors_pkey PRIMARY KEY (id);

CREATE INDEX idx_rag_index_attempt_errors_index_attempt_id ON public.rag_index_attempt_errors USING btree (index_attempt_id);

CREATE INDEX idx_rag_index_attempt_errors_org_id ON public.rag_index_attempt_errors USING btree (org_id);

CREATE INDEX idx_rag_index_attempt_errors_rag_source_id ON public.rag_index_attempt_errors USING btree (rag_source_id);

ALTER TABLE ONLY public.rag_index_attempt_errors
    ADD CONSTRAINT fk_rag_index_attempt_errors_index_attempt FOREIGN KEY (index_attempt_id) REFERENCES public.rag_index_attempts(id) ON DELETE CASCADE;

-- +goose Down
DROP TABLE IF EXISTS public.rag_index_attempt_errors CASCADE;
