-- +goose Up
CREATE TABLE public.team_rag_sources (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    team_id uuid NOT NULL,
    rag_source_id uuid NOT NULL,
    granted_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE ONLY public.team_rag_sources
    ADD CONSTRAINT team_rag_sources_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.team_rag_sources
    ADD CONSTRAINT team_rag_sources_team_source_unique UNIQUE (team_id, rag_source_id);

CREATE INDEX idx_team_rag_sources_org_team ON public.team_rag_sources USING btree (org_id, team_id);

ALTER TABLE ONLY public.team_rag_sources
    ADD CONSTRAINT team_rag_sources_granted_by_fkey FOREIGN KEY (granted_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.team_rag_sources
    ADD CONSTRAINT team_rag_sources_rag_source_id_fkey FOREIGN KEY (rag_source_id) REFERENCES public.rag_sources(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.team_rag_sources
    ADD CONSTRAINT team_rag_sources_team_id_fkey FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;


--
-- PostgreSQL database dump complete
--

-- +goose Down
DROP TABLE IF EXISTS public.team_rag_sources CASCADE;
