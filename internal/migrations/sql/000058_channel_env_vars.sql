-- +goose Up
CREATE TABLE public.channel_env_vars (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    channel_id uuid NOT NULL,
    name text NOT NULL,
    encrypted_value bytea NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    description text DEFAULT ''::text NOT NULL
);

ALTER TABLE ONLY public.channel_env_vars
    ADD CONSTRAINT channel_env_vars_pkey PRIMARY KEY (id);

CREATE INDEX idx_channel_env_vars_channel ON public.channel_env_vars USING btree (channel_id);

CREATE UNIQUE INDEX idx_channel_env_vars_channel_name ON public.channel_env_vars USING btree (channel_id, name);

CREATE INDEX idx_channel_env_vars_org ON public.channel_env_vars USING btree (org_id);

ALTER TABLE ONLY public.channel_env_vars
    ADD CONSTRAINT channel_env_vars_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES public.channels(id) ON DELETE CASCADE;
