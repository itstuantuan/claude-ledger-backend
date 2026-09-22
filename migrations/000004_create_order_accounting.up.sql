CREATE TABLE business_sequences (
    store_id uuid NOT NULL REFERENCES stores(id) ON DELETE RESTRICT,
    business_date date NOT NULL,
    business_type varchar(20) NOT NULL,
    last_value bigint NOT NULL DEFAULT 0 CHECK (last_value >= 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (store_id, business_date, business_type)
);

CREATE TABLE idempotency_records (
    id uuid PRIMARY KEY,
    store_id uuid NOT NULL REFERENCES stores(id) ON DELETE RESTRICT,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    idempotency_key varchar(255) NOT NULL,
    operation varchar(80) NOT NULL,
    request_hash bytea NOT NULL,
    resource_type varchar(80),
    resource_id uuid,
    response_status integer,
    response_data jsonb,
    status text NOT NULL CHECK (status IN ('PROCESSING','COMPLETED','FAILED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    UNIQUE (store_id, idempotency_key)
);
CREATE INDEX idempotency_records_expiry_idx ON idempotency_records (expires_at);

CREATE TABLE orders (
    id uuid PRIMARY KEY,
    store_id uuid NOT NULL REFERENCES stores(id) ON DELETE RESTRICT,
    order_no varchar(40) NOT NULL,
    worker_id uuid NOT NULL REFERENCES workers(id) ON DELETE RESTRICT,
    worker_name_snapshot varchar(80) NOT NULL,
    project_id uuid REFERENCES projects(id) ON DELETE RESTRICT,
    project_name_snapshot varchar(160),
    status text NOT NULL CHECK (status IN ('DRAFT','CONFIRMED','PARTIALLY_PAID','PAID','REVERSED')),
    goods_amount numeric(20,2) NOT NULL CHECK (goods_amount >= 0),
    discount_amount numeric(20,2) NOT NULL CHECK (discount_amount >= 0),
    final_amount numeric(20,2) NOT NULL CHECK (final_amount >= 0),
    payment_amount numeric(20,2) NOT NULL CHECK (payment_amount >= 0),
    prepaid_deduction_amount numeric(20,2) NOT NULL CHECK (prepaid_deduction_amount >= 0),
    added_receivable numeric(20,2) NOT NULL CHECK (added_receivable >= 0),
    returned_amount numeric(20,2) NOT NULL DEFAULT 0 CHECK (returned_amount >= 0),
    settled_amount numeric(20,2) NOT NULL CHECK (settled_amount >= 0),
    outstanding_amount numeric(20,2) NOT NULL CHECK (outstanding_amount >= 0),
    payment_method text CHECK (payment_method IS NULL OR payment_method IN ('WECHAT','ALIPAY','CASH','BANK_CARD','OTHER')),
    remark varchar(500) NOT NULL DEFAULT '',
    occurred_at timestamptz NOT NULL,
    confirmed_at timestamptz NOT NULL,
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT orders_number_unique UNIQUE (store_id, order_no),
    CONSTRAINT orders_amount_check CHECK (goods_amount - discount_amount = final_amount),
    CONSTRAINT orders_settlement_check CHECK (final_amount = payment_amount + prepaid_deduction_amount + added_receivable)
);
CREATE INDEX orders_store_worker_occurred_idx ON orders (store_id, worker_id, occurred_at DESC);
CREATE INDEX orders_store_status_occurred_idx ON orders (store_id, status, occurred_at DESC);
CREATE INDEX orders_store_project_idx ON orders (store_id, project_id);

CREATE TABLE order_items (
    id uuid PRIMARY KEY,
    store_id uuid NOT NULL REFERENCES stores(id) ON DELETE RESTRICT,
    order_id uuid NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    material_id uuid NOT NULL REFERENCES materials(id) ON DELETE RESTRICT,
    material_name_snapshot varchar(160) NOT NULL,
    brand_snapshot varchar(80) NOT NULL,
    specification_snapshot varchar(80) NOT NULL,
    unit_snapshot varchar(30) NOT NULL,
    unit_price numeric(20,2) NOT NULL CHECK (unit_price >= 0),
    quantity numeric(20,3) NOT NULL CHECK (quantity > 0),
    gross_amount numeric(20,2) NOT NULL CHECK (gross_amount >= 0),
    discount_amount numeric(20,2) NOT NULL CHECK (discount_amount >= 0),
    subtotal numeric(20,2) NOT NULL CHECK (subtotal >= 0),
    price_source text NOT NULL CHECK (price_source IN ('MANUAL','CUSTOMER','DEFAULT')),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT order_items_amount_check CHECK (gross_amount - discount_amount = subtotal)
);
CREATE INDEX order_items_order_idx ON order_items (order_id);
CREATE INDEX order_items_store_material_idx ON order_items (store_id, material_id, created_at DESC);

CREATE TABLE prepaid_accounts (
    id uuid PRIMARY KEY,
    store_id uuid NOT NULL REFERENCES stores(id) ON DELETE RESTRICT,
    worker_id uuid NOT NULL REFERENCES workers(id) ON DELETE RESTRICT,
    balance numeric(20,2) NOT NULL DEFAULT 0 CHECK (balance >= 0),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (store_id, worker_id)
);

CREATE TABLE payments (
    id uuid PRIMARY KEY, store_id uuid NOT NULL REFERENCES stores(id) ON DELETE RESTRICT,
    payment_no varchar(40) NOT NULL, worker_id uuid NOT NULL REFERENCES workers(id) ON DELETE RESTRICT,
    order_id uuid REFERENCES orders(id) ON DELETE RESTRICT, amount numeric(20,2) NOT NULL CHECK (amount > 0),
    payment_method text NOT NULL CHECK (payment_method IN ('WECHAT','ALIPAY','CASH','BANK_CARD','OTHER')),
    occurred_at timestamptz NOT NULL, status text NOT NULL CHECK (status IN ('CONFIRMED','REVERSED')),
    remark varchar(500) NOT NULL DEFAULT '', created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(store_id,payment_no)
);

CREATE TABLE ledger_entries (
    id uuid PRIMARY KEY, store_id uuid NOT NULL REFERENCES stores(id) ON DELETE RESTRICT,
    worker_id uuid NOT NULL REFERENCES workers(id) ON DELETE RESTRICT, project_id uuid REFERENCES projects(id) ON DELETE RESTRICT,
    ledger_no varchar(40) NOT NULL, type text NOT NULL CHECK (type IN ('OPENING_BALANCE','ORDER_CHARGE','ORDER_PAYMENT','PREPAID_DEDUCTION','PAYMENT','RETURN_CREDIT','ADJUSTMENT','REVERSAL')),
    signed_amount numeric(20,2) NOT NULL CHECK (signed_amount <> 0), balance_after numeric(20,2) NOT NULL,
    source_type varchar(40) NOT NULL, source_id uuid NOT NULL, source_no varchar(40) NOT NULL,
    reversal_of_id uuid REFERENCES ledger_entries(id) ON DELETE RESTRICT, remark varchar(500) NOT NULL DEFAULT '',
    occurred_at timestamptz NOT NULL, created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT, created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(store_id,ledger_no)
);
CREATE UNIQUE INDEX ledger_entries_reversal_unique ON ledger_entries(reversal_of_id) WHERE reversal_of_id IS NOT NULL;
CREATE INDEX ledger_entries_worker_time_idx ON ledger_entries(store_id,worker_id,occurred_at,id);
CREATE INDEX ledger_entries_source_idx ON ledger_entries(store_id,source_type,source_id);

CREATE TABLE prepaid_transactions (
    id uuid PRIMARY KEY, store_id uuid NOT NULL REFERENCES stores(id) ON DELETE RESTRICT,
    worker_id uuid NOT NULL REFERENCES workers(id) ON DELETE RESTRICT, transaction_no varchar(40) NOT NULL,
    type text NOT NULL CHECK (type IN ('DEPOSIT','DEDUCTION','REFUND','ADJUSTMENT','REVERSAL')),
    signed_amount numeric(20,2) NOT NULL CHECK (signed_amount <> 0), balance_before numeric(20,2) NOT NULL CHECK(balance_before>=0), balance_after numeric(20,2) NOT NULL CHECK(balance_after>=0),
    source_type varchar(40) NOT NULL, source_id uuid NOT NULL, reversal_of_id uuid REFERENCES prepaid_transactions(id) ON DELETE RESTRICT,
    remark varchar(500) NOT NULL DEFAULT '', occurred_at timestamptz NOT NULL, created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT, created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(store_id,transaction_no)
);
CREATE INDEX prepaid_transactions_worker_time_idx ON prepaid_transactions(store_id,worker_id,occurred_at,id);

CREATE TABLE financial_transactions (
    id uuid PRIMARY KEY, store_id uuid NOT NULL REFERENCES stores(id) ON DELETE RESTRICT, transaction_no varchar(40) NOT NULL,
    type text NOT NULL CHECK(type IN ('ORDER_PAYMENT','PAYMENT','PREPAID_DEPOSIT','PREPAID_REFUND','RETURN_REFUND','REVERSAL')),
    direction text NOT NULL CHECK(direction IN ('INCOME','EXPENSE')), amount numeric(20,2) NOT NULL CHECK(amount>0),
    payment_method text NOT NULL CHECK(payment_method IN ('WECHAT','ALIPAY','CASH','BANK_CARD','OTHER')),
    worker_id uuid REFERENCES workers(id) ON DELETE RESTRICT, source_type varchar(40) NOT NULL, source_id uuid NOT NULL, source_no varchar(40) NOT NULL,
    reversal_of_id uuid REFERENCES financial_transactions(id) ON DELETE RESTRICT, occurred_at timestamptz NOT NULL,
    remark varchar(500) NOT NULL DEFAULT '', created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT, created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(store_id,transaction_no)
);
CREATE INDEX financial_transactions_store_time_idx ON financial_transactions(store_id,occurred_at);
CREATE INDEX financial_transactions_source_idx ON financial_transactions(store_id,source_type,source_id);

CREATE TABLE audit_logs (
    id uuid PRIMARY KEY, store_id uuid NOT NULL REFERENCES stores(id) ON DELETE RESTRICT, user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    action varchar(120) NOT NULL, resource_type varchar(80) NOT NULL, resource_id uuid NOT NULL,
    before_data jsonb, after_data jsonb, request_id varchar(100) NOT NULL, ip inet, user_agent text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_logs_resource_idx ON audit_logs(store_id,resource_type,resource_id,created_at);

CREATE OR REPLACE FUNCTION reject_financial_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'immutable financial record'; END $$;
CREATE TRIGGER ledger_entries_immutable BEFORE UPDATE OR DELETE ON ledger_entries FOR EACH ROW EXECUTE FUNCTION reject_financial_mutation();
CREATE TRIGGER prepaid_transactions_immutable BEFORE UPDATE OR DELETE ON prepaid_transactions FOR EACH ROW EXECUTE FUNCTION reject_financial_mutation();
CREATE TRIGGER financial_transactions_immutable BEFORE UPDATE OR DELETE ON financial_transactions FOR EACH ROW EXECUTE FUNCTION reject_financial_mutation();
