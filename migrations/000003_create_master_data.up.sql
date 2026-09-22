CREATE TABLE teams (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    store_id uuid NOT NULL REFERENCES stores(id) ON DELETE RESTRICT,
    name varchar(100) NOT NULL,
    leader_name varchar(80) NOT NULL,
    phone varchar(30) NOT NULL,
    status text NOT NULL DEFAULT 'ACTIVE',
    remark varchar(500) NOT NULL DEFAULT '',
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT teams_status_check CHECK (status IN ('ACTIVE', 'DISABLED')),
    CONSTRAINT teams_version_check CHECK (version > 0),
    CONSTRAINT teams_name_not_blank CHECK (btrim(name) <> '')
);
CREATE INDEX teams_store_status_idx ON teams (store_id, status);
CREATE INDEX teams_store_name_idx ON teams (store_id, name);

CREATE TABLE workers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    store_id uuid NOT NULL REFERENCES stores(id) ON DELETE RESTRICT,
    team_id uuid REFERENCES teams(id) ON DELETE RESTRICT,
    name varchar(80) NOT NULL,
    phone varchar(30) NOT NULL,
    wechat varchar(100),
    address varchar(300) NOT NULL DEFAULT '',
    credit_limit numeric(20,2),
    status text NOT NULL DEFAULT 'ACTIVE',
    remark varchar(500) NOT NULL DEFAULT '',
    material_total numeric(20,2) NOT NULL DEFAULT 0,
    return_total numeric(20,2) NOT NULL DEFAULT 0,
    payment_total numeric(20,2) NOT NULL DEFAULT 0,
    prepaid_balance numeric(20,2) NOT NULL DEFAULT 0,
    current_receivable numeric(20,2) NOT NULL DEFAULT 0,
    last_transaction_at timestamptz,
    version bigint NOT NULL DEFAULT 1,
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT workers_status_check CHECK (status IN ('ACTIVE', 'DISABLED')),
    CONSTRAINT workers_version_check CHECK (version > 0),
    CONSTRAINT workers_credit_limit_check CHECK (credit_limit IS NULL OR credit_limit >= 0),
    CONSTRAINT workers_totals_check CHECK (material_total >= 0 AND return_total >= 0 AND payment_total >= 0 AND prepaid_balance >= 0),
    CONSTRAINT workers_name_not_blank CHECK (btrim(name) <> '')
);
CREATE INDEX workers_store_status_idx ON workers (store_id, status);
CREATE INDEX workers_store_team_idx ON workers (store_id, team_id);
CREATE INDEX workers_store_name_idx ON workers (store_id, name);
CREATE INDEX workers_store_phone_idx ON workers (store_id, phone);
CREATE INDEX workers_store_last_transaction_idx ON workers (store_id, last_transaction_at DESC);

CREATE TABLE projects (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    store_id uuid NOT NULL REFERENCES stores(id) ON DELETE RESTRICT,
    team_id uuid REFERENCES teams(id) ON DELETE RESTRICT,
    name varchar(160) NOT NULL,
    address varchar(300) NOT NULL,
    manager_name varchar(80) NOT NULL,
    contact_phone varchar(30),
    start_date date,
    end_date date,
    status text NOT NULL DEFAULT 'PLANNING',
    remark varchar(500) NOT NULL DEFAULT '',
    material_total numeric(20,2) NOT NULL DEFAULT 0,
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT projects_status_check CHECK (status IN ('PLANNING', 'ACTIVE', 'COMPLETED', 'CANCELLED')),
    CONSTRAINT projects_version_check CHECK (version > 0),
    CONSTRAINT projects_dates_check CHECK (end_date IS NULL OR start_date IS NULL OR end_date >= start_date),
    CONSTRAINT projects_material_total_check CHECK (material_total >= 0),
    CONSTRAINT projects_name_not_blank CHECK (btrim(name) <> '')
);
CREATE INDEX projects_store_status_idx ON projects (store_id, status);
CREATE INDEX projects_store_team_idx ON projects (store_id, team_id);
CREATE INDEX projects_store_name_idx ON projects (store_id, name);

CREATE TABLE project_workers (
    store_id uuid NOT NULL REFERENCES stores(id) ON DELETE RESTRICT,
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
    worker_id uuid NOT NULL REFERENCES workers(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, worker_id)
);
CREATE INDEX project_workers_store_worker_idx ON project_workers (store_id, worker_id);

CREATE TABLE materials (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    store_id uuid NOT NULL REFERENCES stores(id) ON DELETE RESTRICT,
    name varchar(160) NOT NULL,
    category varchar(80) NOT NULL,
    brand varchar(80) NOT NULL,
    specification varchar(80) NOT NULL,
    unit varchar(30) NOT NULL,
    default_sale_price numeric(20,2) NOT NULL,
    cost_price numeric(20,2) NOT NULL,
    status text NOT NULL DEFAULT 'ACTIVE',
    sales_count bigint NOT NULL DEFAULT 0,
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT materials_status_check CHECK (status IN ('ACTIVE', 'DISABLED')),
    CONSTRAINT materials_prices_check CHECK (default_sale_price >= 0 AND cost_price >= 0),
    CONSTRAINT materials_sales_count_check CHECK (sales_count >= 0),
    CONSTRAINT materials_version_check CHECK (version > 0),
    CONSTRAINT materials_name_not_blank CHECK (btrim(name) <> '')
);
CREATE INDEX materials_store_status_idx ON materials (store_id, status);
CREATE INDEX materials_store_category_idx ON materials (store_id, category);
CREATE INDEX materials_store_brand_idx ON materials (store_id, brand);
CREATE INDEX materials_store_name_idx ON materials (store_id, name);

CREATE TABLE customer_prices (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    store_id uuid NOT NULL REFERENCES stores(id) ON DELETE RESTRICT,
    worker_id uuid NOT NULL REFERENCES workers(id) ON DELETE RESTRICT,
    material_id uuid NOT NULL REFERENCES materials(id) ON DELETE RESTRICT,
    price numeric(20,2) NOT NULL,
    effective_from timestamptz NOT NULL,
    effective_to timestamptz,
    status text NOT NULL DEFAULT 'ACTIVE',
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT customer_prices_amount_check CHECK (price >= 0),
    CONSTRAINT customer_prices_status_check CHECK (status IN ('ACTIVE', 'DISABLED')),
    CONSTRAINT customer_prices_dates_check CHECK (effective_to IS NULL OR effective_to > effective_from),
    CONSTRAINT customer_prices_version_check CHECK (version > 0)
);
CREATE UNIQUE INDEX customer_prices_one_current_idx ON customer_prices (store_id, worker_id, material_id) WHERE effective_to IS NULL AND status = 'ACTIVE';
CREATE INDEX customer_prices_lookup_idx ON customer_prices (store_id, worker_id, material_id, effective_from DESC);
