-- +goose Up

ALTER TABLE public.orgs
    ADD COLUMN IF NOT EXISTS billing_currency character varying(3) DEFAULT ''::character varying NOT NULL;

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'orgs_billing_currency_check'
          AND conrelid = 'public.orgs'::regclass
    ) THEN
        ALTER TABLE public.orgs
            ADD CONSTRAINT orgs_billing_currency_check
            CHECK (((billing_currency)::text = ANY ((ARRAY[''::character varying, 'USD'::character varying, 'NGN'::character varying])::text[])));
    END IF;
END $$;
-- +goose StatementEnd

CREATE TABLE IF NOT EXISTS public.billing_payment_methods (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    user_id uuid NOT NULL,
    provider character varying(32) NOT NULL,
    provider_signature character varying(128) NOT NULL,
    encrypted_authorization bytea NOT NULL,
    wrapped_dek bytea NOT NULL,
    card_type character varying(32) DEFAULT ''::character varying NOT NULL,
    last4 character varying(4) DEFAULT ''::character varying NOT NULL,
    exp_month character varying(2) DEFAULT ''::character varying NOT NULL,
    exp_year character varying(4) DEFAULT ''::character varying NOT NULL,
    bank character varying(128) DEFAULT ''::character varying NOT NULL,
    country_code character varying(2) DEFAULT ''::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT billing_payment_methods_pkey PRIMARY KEY (id),
    CONSTRAINT fk_billing_payment_methods_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE,
    CONSTRAINT fk_billing_payment_methods_user FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_billing_payment_methods_org_id ON public.billing_payment_methods USING btree (org_id);
CREATE INDEX IF NOT EXISTS idx_billing_payment_methods_user_id ON public.billing_payment_methods USING btree (user_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_billing_payment_methods_user_signature ON public.billing_payment_methods USING btree (org_id, user_id, provider, provider_signature);

CREATE TABLE IF NOT EXISTS public.credit_purchases (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    created_by_user_id uuid,
    pack_id character varying(32) NOT NULL,
    idempotency_key character varying(64) NOT NULL,
    payment_method_id uuid,
    save_payment_method boolean DEFAULT false NOT NULL,
    provider character varying(32) NOT NULL,
    provider_reference character varying(128) DEFAULT ''::character varying NOT NULL,
    checkout_access_code character varying(128) DEFAULT ''::character varying NOT NULL,
    checkout_url text DEFAULT ''::text NOT NULL,
    status character varying(32) DEFAULT 'pending'::character varying NOT NULL,
    currency character varying(3) NOT NULL,
    subtotal_minor bigint NOT NULL,
    fee_basis_points bigint NOT NULL,
    fee_minor bigint NOT NULL,
    total_minor bigint NOT NULL,
    credits bigint NOT NULL,
    fx_minor_per_usd bigint,
    provider_paid_minor bigint DEFAULT 0 NOT NULL,
    provider_paid_currency character varying(3) DEFAULT ''::character varying NOT NULL,
    paid_at timestamp with time zone,
    credited_at timestamp with time zone,
    failed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT credit_purchases_pkey PRIMARY KEY (id),
    CONSTRAINT credit_purchases_currency_check CHECK (((currency)::text = ANY ((ARRAY['USD'::character varying, 'NGN'::character varying])::text[]))),
    CONSTRAINT credit_purchases_status_check CHECK (((status)::text = ANY ((ARRAY['pending'::character varying, 'paid'::character varying, 'credited'::character varying, 'failed'::character varying, 'reversed'::character varying, 'refunded'::character varying])::text[]))),
    CONSTRAINT credit_purchases_amounts_check CHECK (((subtotal_minor > 0) AND (fee_minor >= 0) AND (total_minor = (subtotal_minor + fee_minor)) AND (credits > 0))),
    CONSTRAINT fk_credit_purchases_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE,
    CONSTRAINT fk_credit_purchases_created_by_user FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL,
    CONSTRAINT fk_credit_purchases_payment_method FOREIGN KEY (payment_method_id) REFERENCES public.billing_payment_methods(id) ON DELETE SET NULL
);

ALTER TABLE public.credit_purchases
    ADD COLUMN IF NOT EXISTS pack_id character varying(32),
    ADD COLUMN IF NOT EXISTS idempotency_key character varying(64),
    ADD COLUMN IF NOT EXISTS payment_method_id uuid,
    ADD COLUMN IF NOT EXISTS save_payment_method boolean DEFAULT false NOT NULL,
    ADD COLUMN IF NOT EXISTS checkout_access_code character varying(128) DEFAULT ''::character varying NOT NULL,
    ADD COLUMN IF NOT EXISTS checkout_url text DEFAULT ''::text NOT NULL;

UPDATE public.credit_purchases
SET pack_id = CASE currency WHEN 'USD' THEN 'usd_legacy' WHEN 'NGN' THEN 'ngn_legacy' ELSE 'legacy' END
WHERE pack_id IS NULL;

UPDATE public.credit_purchases
SET idempotency_key = id::text
WHERE idempotency_key IS NULL;

ALTER TABLE public.credit_purchases
    ALTER COLUMN pack_id SET NOT NULL,
    ALTER COLUMN idempotency_key SET NOT NULL;

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'fk_credit_purchases_payment_method'
          AND conrelid = 'public.credit_purchases'::regclass
    ) THEN
        ALTER TABLE public.credit_purchases
            ADD CONSTRAINT fk_credit_purchases_payment_method
            FOREIGN KEY (payment_method_id) REFERENCES public.billing_payment_methods(id) ON DELETE SET NULL;
    END IF;
END $$;
-- +goose StatementEnd

CREATE INDEX IF NOT EXISTS idx_credit_purchases_created_by_user_id ON public.credit_purchases USING btree (created_by_user_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_credit_purchases_org_id_idempotency_key ON public.credit_purchases USING btree (org_id, idempotency_key);
CREATE INDEX IF NOT EXISTS idx_credit_purchases_payment_method_id ON public.credit_purchases USING btree (payment_method_id);
CREATE INDEX IF NOT EXISTS idx_credit_purchases_org_id ON public.credit_purchases USING btree (org_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_credit_purchases_provider_reference ON public.credit_purchases USING btree (provider, provider_reference) WHERE ((provider_reference)::text <> ''::text);

-- +goose Down

DROP TABLE IF EXISTS public.credit_purchases;
DROP TABLE IF EXISTS public.billing_payment_methods;

ALTER TABLE public.orgs
    DROP CONSTRAINT IF EXISTS orgs_billing_currency_check,
    DROP COLUMN IF EXISTS billing_currency;
