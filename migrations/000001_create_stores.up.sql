CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE stores (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name varchar(100) NOT NULL,
    phone varchar(30),
    address varchar(300),
    timezone varchar(64) NOT NULL DEFAULT 'Asia/Shanghai',
    status text NOT NULL DEFAULT 'ACTIVE',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT stores_status_check CHECK (status IN ('ACTIVE', 'DISABLED')),
    CONSTRAINT stores_timezone_not_blank CHECK (btrim(timezone) <> '')
);
