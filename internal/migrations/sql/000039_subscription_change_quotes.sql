-- +goose Up
CREATE TABLE public.subscription_change_quotes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    subscription_id uuid NOT NULL,
    from_plan_id uuid NOT NULL,
    to_plan_id uuid NOT NULL,
    kind character varying(16) NOT NULL,
    amount_minor bigint NOT NULL,
    currency character varying(8) NOT NULL,
    proration_credit_minor bigint DEFAULT 0 NOT NULL,
    effective_at timestamp with time zone NOT NULL,
    paystack_reference character varying(128),
    expires_at timestamp with time zone NOT NULL,
    consumed_at timestamp with time zone,
    created_at timestamp with time zone
);

ALTER TABLE ONLY public.subscription_change_quotes
    ADD CONSTRAINT subscription_change_quotes_pkey PRIMARY KEY (id);

CREATE INDEX idx_subscription_change_quotes_expires_at ON public.subscription_change_quotes USING btree (expires_at);

CREATE INDEX idx_subscription_change_quotes_org_id ON public.subscription_change_quotes USING btree (org_id);

CREATE UNIQUE INDEX idx_subscription_change_quotes_paystack_reference ON public.subscription_change_quotes USING btree (paystack_reference);

CREATE INDEX idx_subscription_change_quotes_subscription_id ON public.subscription_change_quotes USING btree (subscription_id);

ALTER TABLE ONLY public.subscription_change_quotes
    ADD CONSTRAINT fk_subscription_change_quotes_subscription FOREIGN KEY (subscription_id) REFERENCES public.subscriptions(id) ON DELETE CASCADE;

-- +goose Down
DROP TABLE IF EXISTS public.subscription_change_quotes CASCADE;
