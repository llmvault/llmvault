-- +goose Up
CREATE TABLE public.channel_rag_sources (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    channel_id uuid NOT NULL,
    rag_source_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE ONLY public.channel_rag_sources
    ADD CONSTRAINT channel_rag_sources_pkey PRIMARY KEY (id);

CREATE INDEX idx_channel_rag_sources_channel ON public.channel_rag_sources USING btree (channel_id);

CREATE UNIQUE INDEX idx_channel_rag_sources_channel_source ON public.channel_rag_sources USING btree (channel_id, rag_source_id);

CREATE INDEX idx_channel_rag_sources_org ON public.channel_rag_sources USING btree (org_id);

CREATE INDEX idx_channel_rag_sources_source ON public.channel_rag_sources USING btree (rag_source_id);

ALTER TABLE ONLY public.channel_rag_sources
    ADD CONSTRAINT channel_rag_sources_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES public.channels(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.channel_rag_sources
    ADD CONSTRAINT channel_rag_sources_rag_source_id_fkey FOREIGN KEY (rag_source_id) REFERENCES public.rag_sources(id) ON DELETE CASCADE;

-- +goose Down
DROP TABLE IF EXISTS public.channel_rag_sources CASCADE;
