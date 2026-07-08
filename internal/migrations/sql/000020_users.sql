-- +goose Up
CREATE TABLE public.users (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    email text NOT NULL,
    password_hash text,
    name text,
    avatar_url text,
    email_confirmed_at timestamp with time zone,
    banned_at timestamp with time zone,
    ban_reason text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX idx_users_email ON public.users USING btree (email);

-- +goose Down
DROP TABLE IF EXISTS public.users CASCADE;
