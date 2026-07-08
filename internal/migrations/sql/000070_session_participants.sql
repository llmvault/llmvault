-- +goose Up
CREATE TABLE public.session_participants (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    session_id uuid NOT NULL,
    user_id uuid NOT NULL,
    role text DEFAULT 'collaborator'::text NOT NULL,
    invited_by uuid,
    joined_at timestamp with time zone,
    last_seen_at timestamp with time zone,
    created_at timestamp with time zone
);

ALTER TABLE ONLY public.session_participants
    ADD CONSTRAINT session_participants_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX idx_session_participants_session_user ON public.session_participants USING btree (session_id, user_id);

CREATE INDEX idx_session_participants_user_id ON public.session_participants USING btree (user_id);

ALTER TABLE ONLY public.session_participants
    ADD CONSTRAINT fk_session_participants_inviter FOREIGN KEY (invited_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.session_participants
    ADD CONSTRAINT fk_session_participants_session FOREIGN KEY (session_id) REFERENCES public.sessions(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.session_participants
    ADD CONSTRAINT fk_session_participants_user FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

-- +goose Down
DROP TABLE IF EXISTS public.session_participants CASCADE;
