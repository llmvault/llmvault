-- +goose Up
CREATE TABLE public.rag_embedding_models (
    id text NOT NULL,
    provider text NOT NULL,
    model_name text NOT NULL,
    dimension bigint NOT NULL,
    max_input_tokens bigint NOT NULL,
    dataset_name text NOT NULL,
    query_prefix text,
    passage_prefix text,
    pricing_per_1m_tokens_usd numeric NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY public.rag_embedding_models
    ADD CONSTRAINT rag_embedding_models_pkey PRIMARY KEY (id);

-- +goose Down
DROP TABLE IF EXISTS public.rag_embedding_models CASCADE;
