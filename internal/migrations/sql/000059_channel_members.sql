-- +goose Up
CREATE TABLE public.channel_members (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    channel_id uuid NOT NULL,
    user_id uuid NOT NULL,
    role text DEFAULT 'member'::text NOT NULL,
    created_at timestamp with time zone,
    deactivated_at timestamp with time zone
);

ALTER TABLE ONLY public.channel_members
    ADD CONSTRAINT channel_members_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX idx_channel_members_channel_user ON public.channel_members USING btree (channel_id, user_id);

CREATE INDEX idx_channel_members_user_id ON public.channel_members USING btree (user_id);

ALTER TABLE ONLY public.channel_members
    ADD CONSTRAINT fk_channel_members_channel FOREIGN KEY (channel_id) REFERENCES public.channels(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.channel_members
    ADD CONSTRAINT fk_channel_members_user FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;
