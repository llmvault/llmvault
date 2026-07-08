-- +goose Up
ALTER TABLE ONLY public.app_versions
    ADD CONSTRAINT app_versions_app_id_fkey FOREIGN KEY (app_id) REFERENCES public.apps(id) ON DELETE CASCADE;
