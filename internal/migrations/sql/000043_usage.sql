-- +goose Up
CREATE TABLE public.usage (
    id bigint NOT NULL,
    org_id uuid NOT NULL,
    credential_id uuid NOT NULL,
    request_count bigint DEFAULT 0 NOT NULL,
    period_start timestamp with time zone NOT NULL,
    period_end timestamp with time zone NOT NULL,
    created_at timestamp with time zone
);

CREATE SEQUENCE public.usage_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.usage_id_seq OWNED BY public.usage.id;

ALTER TABLE ONLY public.usage ALTER COLUMN id SET DEFAULT nextval('public.usage_id_seq'::regclass);

ALTER TABLE ONLY public.usage
    ADD CONSTRAINT usage_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX idx_usage_unique ON public.usage USING btree (org_id, credential_id, period_start);

ALTER TABLE ONLY public.usage
    ADD CONSTRAINT fk_usage_credential FOREIGN KEY (credential_id) REFERENCES public.credentials(id);

ALTER TABLE ONLY public.usage
    ADD CONSTRAINT fk_usage_org FOREIGN KEY (org_id) REFERENCES public.orgs(id);

-- +goose Down
DROP TABLE IF EXISTS public.usage CASCADE;
