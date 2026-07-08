-- +goose Up
CREATE TABLE public.rag_search_settings (
    org_id uuid NOT NULL,
    embedding_model_id character varying(128) NOT NULL,
    embedding_dim bigint NOT NULL,
    "normalize" boolean DEFAULT true NOT NULL,
    query_prefix text,
    passage_prefix text,
    embedding_precision character varying(16) DEFAULT 'float'::character varying NOT NULL,
    reduced_dimension integer,
    multipass_indexing boolean DEFAULT true NOT NULL,
    reranker_model_id character varying(128),
    hybrid_alpha double precision DEFAULT 0.7 NOT NULL,
    index_name character varying(256) NOT NULL,
    enable_contextual_rag boolean DEFAULT false NOT NULL,
    contextual_ragllm_name character varying(128),
    contextual_ragllm_provider character varying(64),
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY public.rag_search_settings
    ADD CONSTRAINT rag_search_settings_pkey PRIMARY KEY (org_id);

CREATE INDEX idx_rag_search_settings_embedding_model_id ON public.rag_search_settings USING btree (embedding_model_id);

-- +goose Down
DROP TABLE IF EXISTS public.rag_search_settings CASCADE;
