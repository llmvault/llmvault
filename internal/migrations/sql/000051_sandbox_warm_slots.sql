-- +goose Up
CREATE TABLE public.sandbox_warm_slots (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    provider_id text NOT NULL,
    mode text NOT NULL,
    status text DEFAULT 'warming'::text NOT NULL,
    external_id text NOT NULL,
    endpoint_url text NOT NULL,
    runtime_image text NOT NULL,
    runtime_port integer DEFAULT 7080 NOT NULL,
    region text DEFAULT ''::text NOT NULL,
    claimed_sandbox_id uuid,
    encrypted_runtime_secret bytea NOT NULL,
    error_message text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    image_kind text DEFAULT 'default'::text NOT NULL,
    sandbox_size text DEFAULT 'small'::text NOT NULL,
    cpu integer DEFAULT 0 NOT NULL,
    memory integer DEFAULT 0 NOT NULL,
    disk integer DEFAULT 0 NOT NULL
);

ALTER TABLE ONLY public.sandbox_warm_slots
    ADD CONSTRAINT sandbox_warm_slots_pkey PRIMARY KEY (id);

CREATE INDEX idx_sandbox_warm_slots_claimed_sandbox_id ON public.sandbox_warm_slots USING btree (claimed_sandbox_id);

CREATE INDEX idx_sandbox_warm_slots_pool_profile_status ON public.sandbox_warm_slots USING btree (provider_id, mode, image_kind, runtime_image, sandbox_size, cpu, memory, disk, status, created_at);

CREATE INDEX idx_sandbox_warm_slots_pool_status ON public.sandbox_warm_slots USING btree (provider_id, mode, status, created_at);

CREATE UNIQUE INDEX idx_sandbox_warm_slots_provider_external ON public.sandbox_warm_slots USING btree (provider_id, external_id);

ALTER TABLE ONLY public.sandbox_warm_slots
    ADD CONSTRAINT fk_sandbox_warm_slots_claimed_sandbox FOREIGN KEY (claimed_sandbox_id) REFERENCES public.sandboxes(id) ON DELETE SET NULL;

-- +goose Down
DROP TABLE IF EXISTS public.sandbox_warm_slots CASCADE;
