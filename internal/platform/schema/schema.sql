-- Credits & Bundles MVP schema. Recreated on every reset.
-- Every entity is keyed by uuid; human-readable codes/handles are separate UNIQUE columns used by seeds and URLs.
DROP SCHEMA IF EXISTS public CASCADE;
CREATE SCHEMA public;

-- Application service
CREATE TABLE applications (
  id     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code   text NOT NULL UNIQUE,
  vendor text NOT NULL,
  name   text NOT NULL
);

-- Catalog service
CREATE TABLE plans (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code            text NOT NULL UNIQUE,
  application_id  uuid NOT NULL REFERENCES applications(id),
  name            text NOT NULL,
  slot_count      int  NOT NULL,
  licensing_model text NOT NULL
);

CREATE TABLE credit_packages (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code          text NOT NULL UNIQUE,
  name          text NOT NULL,
  credits       bigint NOT NULL CHECK (credits > 0),
  validity_days int NOT NULL,
  application_id uuid                          -- set: an in-app pack, credits usable only in this app
);

CREATE TABLE offerings (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code           text NOT NULL UNIQUE,
  name           text NOT NULL,
  kind           text NOT NULL CHECK (kind IN ('single','bundle','credit_pack')),
  price_minor    bigint NOT NULL,
  currency       text NOT NULL,
  billing_period text NOT NULL CHECK (billing_period IN ('month','one_time')),
  trial_days     int NOT NULL DEFAULT 0,
  grace_days     int NOT NULL DEFAULT 0,
  status         text NOT NULL DEFAULT 'active'
);

CREATE TABLE offering_items (
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code              text NOT NULL UNIQUE,
  offering_id       uuid NOT NULL REFERENCES offerings(id),
  item_type         text NOT NULL CHECK (item_type IN ('plan','credit_package')),
  plan_id           uuid REFERENCES plans(id),
  credit_package_id uuid REFERENCES credit_packages(id),
  quantity          int NOT NULL DEFAULT 1,
  allocation_weight bigint NOT NULL, -- share of offering price, minor units
  CHECK ((plan_id IS NOT NULL) <> (credit_package_id IS NOT NULL))
);

CREATE TABLE action_prices (
  application_id uuid NOT NULL REFERENCES applications(id),
  action_code    text NOT NULL,
  credits        bigint NOT NULL CHECK (credits > 0),
  valid_from     timestamptz NOT NULL,
  PRIMARY KEY (application_id, action_code, valid_from)
);

-- Customer service
CREATE TABLE customers (
  id     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  handle text NOT NULL UNIQUE,
  email  text NOT NULL UNIQUE,
  name   text NOT NULL
);

-- Billing service (simulated: owned by the Billing team in the real system)
CREATE TABLE subscriptions (
  id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  provider_sub_ref     text NOT NULL UNIQUE,
  customer_id          uuid NOT NULL REFERENCES customers(id),
  offering_id          uuid NOT NULL REFERENCES offerings(id),
  kind                 text NOT NULL CHECK (kind IN ('recurring','one_time')),
  status               text NOT NULL CHECK (status IN ('created','active','past_due','canceled')),
  current_period_start timestamptz,
  current_period_end   timestamptz,
  created_at           timestamptz NOT NULL
);

CREATE TABLE payments (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  subscription_id uuid NOT NULL REFERENCES subscriptions(id),
  payment_ref     text NOT NULL UNIQUE,
  amount_minor    bigint NOT NULL,
  currency        text NOT NULL,
  period_start    timestamptz NOT NULL,
  period_end      timestamptz NOT NULL,
  status          text NOT NULL CHECK (status IN ('succeeded','refunded','charged_back')),
  refunded_minor  bigint NOT NULL DEFAULT 0,
  created_at      timestamptz NOT NULL
);

-- License service
CREATE TABLE licenses (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  subscription_id  uuid NOT NULL REFERENCES subscriptions(id),
  customer_id      uuid NOT NULL REFERENCES customers(id),
  offering_item_id uuid NOT NULL REFERENCES offering_items(id),
  application_id   uuid NOT NULL REFERENCES applications(id),
  key              text NOT NULL UNIQUE,
  status           text NOT NULL CHECK (status IN ('active','revoked')),
  due_date         timestamptz NOT NULL,
  UNIQUE (subscription_id, offering_item_id)
);
CREATE INDEX IF NOT EXISTS licenses_customer_app ON licenses (customer_id, application_id) WHERE status = 'active';

-- Credits service
CREATE TABLE credit_accounts (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  customer_id uuid NOT NULL UNIQUE REFERENCES customers(id),
  status      text NOT NULL DEFAULT 'active' CHECK (status IN ('active','blocked'))
);

CREATE TABLE credit_lots (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id       uuid NOT NULL REFERENCES credit_accounts(id),
  source_type      text NOT NULL CHECK (source_type IN ('bundle','subscription','pack','promo')),
  payment_id       uuid REFERENCES payments(id),
  offering_item_id uuid REFERENCES offering_items(id),
  subscription_id  uuid REFERENCES subscriptions(id),
  label            text NOT NULL,
  scope            text NOT NULL CHECK (scope IN ('platform','bundle')),
  kind             text NOT NULL CHECK (kind IN ('paid','bonus')),
  granted          bigint NOT NULL,
  remaining        bigint NOT NULL CHECK (remaining >= 0),
  expires_at       timestamptz NOT NULL,
  revoked_at       timestamptz,
  created_at       timestamptz NOT NULL,
  UNIQUE (payment_id, offering_item_id)
);

CREATE TABLE lot_scope_apps (
  lot_id         uuid NOT NULL REFERENCES credit_lots(id),
  application_id uuid NOT NULL REFERENCES applications(id),
  PRIMARY KEY (lot_id, application_id)
);

CREATE TABLE credit_charges (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id      uuid NOT NULL REFERENCES credit_accounts(id),
  application_id  uuid NOT NULL REFERENCES applications(id),
  action_code     text NOT NULL,
  amount          bigint NOT NULL,
  status          text NOT NULL CHECK (status IN ('in_progress','completed','refunded')),
  idempotency_key text NOT NULL,
  request_hash    text NOT NULL,
  result          jsonb,
  cost_minor      bigint,
  created_at      timestamptz NOT NULL,
  UNIQUE (account_id, idempotency_key)
);

-- append-only
CREATE TABLE credit_ledger (
  id         bigserial PRIMARY KEY,
  account_id uuid NOT NULL REFERENCES credit_accounts(id),
  lot_id     uuid NOT NULL REFERENCES credit_lots(id),
  charge_id  uuid REFERENCES credit_charges(id),
  type       text NOT NULL CHECK (type IN ('grant','charge','refund','expire','revoke')),
  delta      bigint NOT NULL,
  note       text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL
);

-- Webhook inbox (dedup + audit) — Billing team's side, simulated
CREATE TABLE webhook_events (
  event_id         text PRIMARY KEY,
  type             text NOT NULL,
  provider_sub_ref text NOT NULL,
  payload          jsonb NOT NULL,
  status           text NOT NULL CHECK (status IN ('applied','duplicate','rejected','parked')),
  reason           text NOT NULL DEFAULT '',
  received_at      timestamptz NOT NULL
);

-- Mock payment provider's own records (the "other side" for reconciliation)
CREATE TABLE provider_txns (
  payment_ref      text PRIMARY KEY,
  provider_sub_ref text NOT NULL,
  customer_id      uuid NOT NULL,
  offering_id      uuid NOT NULL,
  amount_minor     bigint NOT NULL,
  currency         text NOT NULL,
  status           text NOT NULL,
  refunded_minor   bigint NOT NULL DEFAULT 0,
  created_at       timestamptz NOT NULL
);

CREATE TABLE sim_clock (
  id             int PRIMARY KEY,
  offset_seconds bigint NOT NULL
);

ALTER TABLE credit_packages ADD FOREIGN KEY (application_id) REFERENCES applications(id);

-- ---------- seed ----------
INSERT INTO sim_clock VALUES (1, 0);

INSERT INTO applications (code, vendor, name) VALUES
  ('cleanbox', 'Northwind',     'CleanBox'),
  ('appshop',    'Pixel Labs', 'App Shop'),
  ('retouch',    'Lumen',      'Face Retouch'),
  ('notes',      'Inkwell',    'Notes AI');

INSERT INTO plans (code, application_id, name, slot_count, licensing_model)
SELECT p.code, a.id, p.name, p.slots, p.model FROM (VALUES
  ('plan_cmm',     'cleanbox', 'CleanBox 1 Mac', 1, 'per_device'),
  ('plan_appshop', 'appshop',    'App Shop',         2, 'per_device'),
  ('plan_retouch', 'retouch',    'Face Retouch',     1, 'per_user'),
  ('plan_notes',   'notes',      'Notes AI',         3, 'per_user')
) AS p(code, app, name, slots, model) JOIN applications a ON a.code = p.app;

INSERT INTO credit_packages (code, name, credits, validity_days) VALUES
  ('pkg_design_10k', 'Design+ monthly credits', 10000, 30),
  ('pkg_500',        'Credit pack 500',           500, 365),
  ('pkg_cmm_300',    'CleanBox AI monthly',     300, 30),
  ('pkg_appshop_1k', 'App Shop Pro monthly',     1000, 30);
INSERT INTO credit_packages (code, name, credits, validity_days, application_id)
SELECT 'pkg_appshop_300', 'App Shop credits 300', 300, 365, id FROM applications WHERE code = 'appshop';

INSERT INTO offerings (code, name, kind, price_minor, currency, billing_period, trial_days, grace_days) VALUES
  ('off_cmm',         'CleanBox monthly',      'single',      2000, 'USD', 'month',    7, 3),
  ('off_appshop',     'App Shop monthly',        'single',      1500, 'USD', 'month',    0, 3),
  ('off_design',      'Design+ bundle',          'bundle',      3000, 'USD', 'month',    7, 3),
  ('off_pack500',     'Credit pack 500',         'credit_pack',  500, 'USD', 'one_time', 0, 0),
  ('off_cmm_ai',      'CleanBox + AI monthly', 'single',      2500, 'USD', 'month',    7, 3),
  ('off_appshop_pro', 'App Shop Pro monthly',    'single',      2500, 'USD', 'month',    0, 3),
  ('off_appshop_cr300', 'App Shop credits 300',  'credit_pack',  300, 'USD', 'one_time', 0, 0);

INSERT INTO offering_items (code, offering_id, item_type, plan_id, credit_package_id, quantity, allocation_weight)
SELECT i.code, o.id, i.type, p.id, cp.id, 1, i.weight FROM (VALUES
  ('oi_cmm',              'off_cmm',         'plan',           'plan_cmm',     NULL,             2000),
  ('oi_appshop',          'off_appshop',     'plan',           'plan_appshop', NULL,             1500),
  ('oi_design_appshop',   'off_design',      'plan',           'plan_appshop', NULL,              800),
  ('oi_design_retouch',   'off_design',      'plan',           'plan_retouch', NULL,              800),
  ('oi_design_notes',     'off_design',      'plan',           'plan_notes',   NULL,              800),
  ('oi_design_credits',   'off_design',      'credit_package', NULL,           'pkg_design_10k',  600),
  ('oi_pack500',          'off_pack500',     'credit_package', NULL,           'pkg_500',         500),
  ('oi_cmm_ai_plan',      'off_cmm_ai',      'plan',           'plan_cmm',     NULL,             2000),
  ('oi_cmm_ai_credits',   'off_cmm_ai',      'credit_package', NULL,           'pkg_cmm_300',     500),
  ('oi_appshop_pro_plan', 'off_appshop_pro', 'plan',           'plan_appshop', NULL,             1500),
  ('oi_appshop_pro_cr',   'off_appshop_pro', 'credit_package', NULL,           'pkg_appshop_1k', 1000),
  ('oi_appshop_cr300',    'off_appshop_cr300', 'credit_package', NULL,         'pkg_appshop_300', 300)
) AS i(code, off, type, plan, pkg, weight)
JOIN offerings o ON o.code = i.off
LEFT JOIN plans p ON p.code = i.plan
LEFT JOIN credit_packages cp ON cp.code = i.pkg;

INSERT INTO action_prices (application_id, action_code, credits, valid_from)
SELECT a.id, x.action, x.credits, '2026-01-01' FROM (VALUES
  ('appshop',    'ai.generate',  80),
  ('appshop',    'ai.upscale',   40),
  ('retouch',    'ai.retouch',   50),
  ('retouch',    'ai.bg_remove', 15),
  ('notes',      'ai.summarize', 20),
  ('notes',      'ai.translate', 35),
  ('cleanbox', 'ai.scan',      30),
  ('cleanbox', 'ai.explain',   10)
) AS x(app, action, credits) JOIN applications a ON a.code = x.app;

INSERT INTO customers (handle, email, name) VALUES
  ('alice', 'alice@example.com', 'Alice'),
  ('bob',   'bob@example.com',   'Bob');

INSERT INTO credit_accounts (customer_id) SELECT id FROM customers;
