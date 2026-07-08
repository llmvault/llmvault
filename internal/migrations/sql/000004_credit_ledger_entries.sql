-- +goose Up
CREATE TABLE public.credit_ledger_entries (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    amount bigint NOT NULL,
    reason character varying(64) NOT NULL,
    ref_type character varying(64),
    ref_id character varying(64),
    expires_at timestamp with time zone,
    created_at timestamp with time zone
);

ALTER TABLE ONLY public.credit_ledger_entries
    ADD CONSTRAINT credit_ledger_entries_pkey PRIMARY KEY (id);

CREATE INDEX idx_credit_ledger_entries_expires_at ON public.credit_ledger_entries USING btree (expires_at);

CREATE UNIQUE INDEX idx_credit_ledger_entries_idem ON public.credit_ledger_entries USING btree (org_id, reason, ref_type, ref_id) WHERE ((ref_id)::text <> ''::text);

CREATE INDEX idx_credit_ledger_entries_org_id ON public.credit_ledger_entries USING btree (org_id);

CREATE INDEX idx_credit_ledger_entries_ref_id ON public.credit_ledger_entries USING btree (ref_id);
