-- +goose Up

ALTER TABLE public.billing_payment_methods
    ADD COLUMN IF NOT EXISTS currency character varying(3);

UPDATE public.billing_payment_methods AS method
SET currency = org.billing_currency
FROM public.orgs AS org
WHERE method.org_id = org.id
  AND method.currency IS NULL
  AND org.billing_currency IN ('USD', 'NGN');

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM public.billing_payment_methods
        WHERE currency IS NULL
    ) THEN
        RAISE EXCEPTION 'cannot migrate billing payment method without an existing billing currency';
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE public.billing_payment_methods
    ALTER COLUMN currency SET NOT NULL,
    DROP CONSTRAINT IF EXISTS billing_payment_methods_currency_check,
    ADD CONSTRAINT billing_payment_methods_currency_check
        CHECK (currency IN ('USD', 'NGN'));

DROP INDEX IF EXISTS public.idx_billing_payment_methods_user_signature;

CREATE UNIQUE INDEX idx_billing_payment_methods_user_signature_currency
    ON public.billing_payment_methods
    USING btree (org_id, user_id, provider, provider_signature, currency);

ALTER TABLE public.orgs
    DROP CONSTRAINT IF EXISTS orgs_billing_currency_check,
    DROP COLUMN IF EXISTS billing_currency;

-- +goose Down

ALTER TABLE public.orgs
    ADD COLUMN IF NOT EXISTS billing_currency character varying(3) DEFAULT ''::character varying NOT NULL;

UPDATE public.orgs AS org
SET billing_currency = latest.currency
FROM (
    SELECT DISTINCT ON (org_id) org_id, currency
    FROM public.credit_purchases
    ORDER BY org_id, created_at DESC
) AS latest
WHERE org.id = latest.org_id;

ALTER TABLE public.orgs
    ADD CONSTRAINT orgs_billing_currency_check
        CHECK (billing_currency IN ('', 'USD', 'NGN'));

DROP INDEX IF EXISTS public.idx_billing_payment_methods_user_signature_currency;

-- Retain one authorization if the same card was saved independently in both currencies.
DELETE FROM public.billing_payment_methods AS duplicate
USING public.billing_payment_methods AS retained
WHERE duplicate.org_id = retained.org_id
  AND duplicate.user_id = retained.user_id
  AND duplicate.provider = retained.provider
  AND duplicate.provider_signature = retained.provider_signature
  AND (duplicate.created_at, duplicate.id) < (retained.created_at, retained.id);

ALTER TABLE public.billing_payment_methods
    DROP CONSTRAINT IF EXISTS billing_payment_methods_currency_check,
    DROP COLUMN IF EXISTS currency;

CREATE UNIQUE INDEX idx_billing_payment_methods_user_signature
    ON public.billing_payment_methods
    USING btree (org_id, user_id, provider, provider_signature);
