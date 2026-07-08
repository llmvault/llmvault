-- +goose Up
CREATE TABLE public.audit_log (
    id bigint NOT NULL,
    org_id uuid NOT NULL,
    credential_id uuid,
    action text NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb,
    ip_address inet,
    created_at timestamp with time zone
);

CREATE SEQUENCE public.audit_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.audit_log_id_seq OWNED BY public.audit_log.id;

ALTER TABLE ONLY public.audit_log ALTER COLUMN id SET DEFAULT nextval('public.audit_log_id_seq'::regclass);

ALTER TABLE ONLY public.audit_log
    ADD CONSTRAINT audit_log_pkey PRIMARY KEY (id);

CREATE INDEX idx_audit_credential ON public.audit_log USING btree (credential_id);

CREATE INDEX idx_audit_org_created ON public.audit_log USING btree (org_id, created_at);
