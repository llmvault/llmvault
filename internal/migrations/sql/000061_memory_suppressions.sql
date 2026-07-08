-- +goose Up
CREATE TABLE public.memory_suppressions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    channel_id uuid,
    content_fingerprint text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE ONLY public.memory_suppressions
    ADD CONSTRAINT memory_suppressions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.memory_suppressions
    ADD CONSTRAINT memory_suppressions_unique UNIQUE (org_id, channel_id, content_fingerprint);

ALTER TABLE ONLY public.memory_suppressions
    ADD CONSTRAINT fk_memory_suppressions_channel FOREIGN KEY (channel_id) REFERENCES public.channels(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.memory_suppressions
    ADD CONSTRAINT fk_memory_suppressions_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;
