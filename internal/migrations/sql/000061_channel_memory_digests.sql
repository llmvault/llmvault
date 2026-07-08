-- +goose Up
CREATE TABLE public.channel_memory_digests (
    channel_id uuid NOT NULL,
    org_id uuid NOT NULL,
    content text NOT NULL,
    observation_count integer DEFAULT 0 NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE ONLY public.channel_memory_digests
    ADD CONSTRAINT channel_memory_digests_pkey PRIMARY KEY (channel_id);

ALTER TABLE ONLY public.channel_memory_digests
    ADD CONSTRAINT fk_channel_memory_digests_channel FOREIGN KEY (channel_id) REFERENCES public.channels(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.channel_memory_digests
    ADD CONSTRAINT fk_channel_memory_digests_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

-- +goose Down
DROP TABLE IF EXISTS public.channel_memory_digests CASCADE;
