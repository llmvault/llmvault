-- +goose Up
ALTER TABLE public.generations ADD COLUMN session_id uuid;
CREATE INDEX idx_gen_session_id ON public.generations USING btree (session_id) WHERE session_id IS NOT NULL;
