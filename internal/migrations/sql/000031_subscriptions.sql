-- +goose Up
CREATE TABLE public.subscriptions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    plan_id uuid NOT NULL,
    provider character varying(32) NOT NULL,
    external_customer_id character varying(128) NOT NULL,
    status character varying(32) DEFAULT 'active'::character varying NOT NULL,
    current_period_start timestamp with time zone,
    current_period_end timestamp with time zone,
    canceled_at timestamp with time zone,
    cancel_at_period_end boolean DEFAULT false NOT NULL,
    pending_plan_id uuid,
    pending_change_at timestamp with time zone,
    renewal_attempts bigint DEFAULT 0 NOT NULL,
    last_renewal_attempt_at timestamp with time zone,
    last_renewal_error character varying(512) DEFAULT ''::character varying NOT NULL,
    payment_channel character varying(16) DEFAULT ''::character varying NOT NULL,
    payment_bank_name character varying(64) DEFAULT ''::character varying NOT NULL,
    payment_account_name character varying(128) DEFAULT ''::character varying NOT NULL,
    last_charge_reference character varying(128) DEFAULT ''::character varying NOT NULL,
    last_charge_amount bigint DEFAULT 0 NOT NULL,
    last_charged_at timestamp with time zone,
    card_last4 character varying(4) DEFAULT ''::character varying NOT NULL,
    card_brand character varying(32) DEFAULT ''::character varying NOT NULL,
    card_exp_month character varying(2) DEFAULT ''::character varying NOT NULL,
    card_exp_year character varying(4) DEFAULT ''::character varying NOT NULL,
    authorization_code character varying(128) DEFAULT ''::character varying NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY public.subscriptions
    ADD CONSTRAINT subscriptions_pkey PRIMARY KEY (id);

CREATE INDEX idx_subscriptions_external_customer_id ON public.subscriptions USING btree (external_customer_id);

CREATE INDEX idx_subscriptions_org_id ON public.subscriptions USING btree (org_id);

CREATE INDEX idx_subscriptions_plan_id ON public.subscriptions USING btree (plan_id);

ALTER TABLE ONLY public.subscriptions
    ADD CONSTRAINT fk_subscriptions_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.subscriptions
    ADD CONSTRAINT fk_subscriptions_pending_plan FOREIGN KEY (pending_plan_id) REFERENCES public.plans(id);

ALTER TABLE ONLY public.subscriptions
    ADD CONSTRAINT fk_subscriptions_plan FOREIGN KEY (plan_id) REFERENCES public.plans(id);

-- +goose Down
DROP TABLE IF EXISTS public.subscriptions CASCADE;
