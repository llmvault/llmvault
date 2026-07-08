-- +goose Up
CREATE TABLE public.canvas_artifact_files (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    canvas_artifact_id uuid NOT NULL,
    path text NOT NULL,
    role text DEFAULT ''::text NOT NULL,
    content_type text DEFAULT ''::text NOT NULL,
    object_key text NOT NULL,
    size_bytes bigint DEFAULT 0 NOT NULL,
    sha256 text DEFAULT ''::text NOT NULL,
    archived_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT canvas_artifact_files_size_check CHECK ((size_bytes >= 0))
);

ALTER TABLE ONLY public.canvas_artifact_files
    ADD CONSTRAINT canvas_artifact_files_pkey PRIMARY KEY (id);

CREATE INDEX idx_canvas_artifact_files_archived_at ON public.canvas_artifact_files USING btree (archived_at);

CREATE UNIQUE INDEX idx_canvas_artifact_files_artifact_path_active ON public.canvas_artifact_files USING btree (canvas_artifact_id, path) WHERE (archived_at IS NULL);

CREATE INDEX idx_canvas_artifact_files_object_key ON public.canvas_artifact_files USING btree (object_key);

CREATE INDEX idx_canvas_artifact_files_org ON public.canvas_artifact_files USING btree (org_id);

ALTER TABLE ONLY public.canvas_artifact_files
    ADD CONSTRAINT canvas_artifact_files_canvas_artifact_id_fkey FOREIGN KEY (canvas_artifact_id) REFERENCES public.canvas_artifacts(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.canvas_artifact_files
    ADD CONSTRAINT canvas_artifact_files_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

-- +goose Down
DROP TABLE IF EXISTS public.canvas_artifact_files CASCADE;
