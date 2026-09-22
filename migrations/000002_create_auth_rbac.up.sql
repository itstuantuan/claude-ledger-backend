CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    store_id uuid NOT NULL REFERENCES stores(id) ON DELETE RESTRICT,
    account varchar(80) NOT NULL UNIQUE,
    password_hash text NOT NULL,
    name varchar(80) NOT NULL,
    phone varchar(30) NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'ACTIVE',
    auth_version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT users_status_check CHECK (status IN ('ACTIVE', 'DISABLED')),
    CONSTRAINT users_auth_version_check CHECK (auth_version > 0),
    CONSTRAINT users_account_not_blank CHECK (btrim(account) <> ''),
    CONSTRAINT users_store_account_unique UNIQUE (store_id, account)
);

CREATE INDEX users_store_status_idx ON users (store_id, status);

CREATE TABLE roles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    store_id uuid NOT NULL REFERENCES stores(id) ON DELETE RESTRICT,
    code varchar(80) NOT NULL,
    name varchar(100) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT roles_store_code_unique UNIQUE (store_id, code),
    CONSTRAINT roles_code_check CHECK (code IN ('OWNER', 'FINANCE', 'CLERK'))
);

CREATE TABLE permissions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code varchar(100) NOT NULL UNIQUE,
    description text NOT NULL DEFAULT ''
);

CREATE TABLE user_roles (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    role_id uuid NOT NULL REFERENCES roles(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, role_id),
    CONSTRAINT user_roles_single_role_unique UNIQUE (user_id)
);

CREATE INDEX user_roles_role_idx ON user_roles (role_id);

CREATE TABLE role_permissions (
    role_id uuid NOT NULL REFERENCES roles(id) ON DELETE RESTRICT,
    permission_id uuid NOT NULL REFERENCES permissions(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (role_id, permission_id)
);

CREATE INDEX role_permissions_permission_idx ON role_permissions (permission_id);

CREATE TABLE refresh_sessions (
    id uuid PRIMARY KEY,
    family_id uuid NOT NULL REFERENCES refresh_sessions(id) ON DELETE RESTRICT,
    store_id uuid NOT NULL REFERENCES stores(id) ON DELETE RESTRICT,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    token_hash bytea NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    last_used_at timestamptz,
    replaced_by_id uuid REFERENCES refresh_sessions(id) ON DELETE RESTRICT,
    user_agent text NOT NULL DEFAULT '',
    ip inet,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT refresh_sessions_expiry_check CHECK (expires_at > created_at)
);

CREATE INDEX refresh_sessions_user_expiry_idx ON refresh_sessions (user_id, expires_at DESC);
CREATE INDEX refresh_sessions_family_idx ON refresh_sessions (family_id);
CREATE INDEX refresh_sessions_active_expiry_idx ON refresh_sessions (expires_at) WHERE revoked_at IS NULL;

INSERT INTO permissions (code, description) VALUES
('workers:read', '查看油漆工、施工队与工地'),
('workers:write', '维护油漆工、施工队与工地'),
('materials:read', '查看材料和客户价格'),
('materials:write', '维护材料和客户价格'),
('orders:create', '创建用料单'),
('returns:request', '发起退料申请'),
('returns:confirm', '确认退料'),
('returns:manual', '创建无原单退料'),
('finance:read', '查看财务记录'),
('payments:create', '登记客户收款'),
('prepaid:deposit', '登记预存充值'),
('ledger:adjust', '进行账务调整'),
('reconciliation:read', '查看往来账与对账单'),
('reports:read', '查看经营统计'),
('system:manage', '管理用户、角色与系统配置');
