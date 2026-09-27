ALTER TABLE orders ADD COLUMN version bigint NOT NULL DEFAULT 1 CHECK (version > 0);
ALTER TABLE orders DROP CONSTRAINT orders_settlement_check;
ALTER TABLE orders ADD CONSTRAINT orders_balance_check
    CHECK (final_amount - returned_amount = settled_amount + outstanding_amount);

CREATE TABLE order_adjustments (
    id uuid PRIMARY KEY,
    store_id uuid NOT NULL REFERENCES stores(id) ON DELETE RESTRICT,
    order_id uuid NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    adjustment_no varchar(40) NOT NULL,
    type text NOT NULL CHECK (type IN ('SUPPLEMENT','RETURN')),
    goods_amount numeric(20,2) NOT NULL CHECK (goods_amount > 0),
    discount_amount numeric(20,2) NOT NULL DEFAULT 0 CHECK (discount_amount >= 0),
    final_amount numeric(20,2) NOT NULL CHECK (final_amount > 0),
    remark varchar(500) NOT NULL DEFAULT '',
    occurred_at timestamptz NOT NULL,
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT order_adjustments_number_unique UNIQUE (store_id, adjustment_no),
    CONSTRAINT order_adjustments_amount_check CHECK (goods_amount - discount_amount = final_amount)
);
CREATE INDEX order_adjustments_order_time_idx ON order_adjustments(store_id, order_id, occurred_at, created_at);

CREATE TABLE order_adjustment_items (
    id uuid PRIMARY KEY,
    store_id uuid NOT NULL REFERENCES stores(id) ON DELETE RESTRICT,
    adjustment_id uuid NOT NULL REFERENCES order_adjustments(id) ON DELETE RESTRICT,
    order_id uuid NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    material_id uuid NOT NULL REFERENCES materials(id) ON DELETE RESTRICT,
    material_name_snapshot varchar(160) NOT NULL,
    brand_snapshot varchar(80) NOT NULL,
    specification_snapshot varchar(80) NOT NULL,
    unit_snapshot varchar(30) NOT NULL,
    unit_price numeric(20,2) NOT NULL CHECK (unit_price >= 0),
    quantity numeric(20,3) NOT NULL CHECK (quantity > 0),
    gross_amount numeric(20,2) NOT NULL CHECK (gross_amount > 0),
    discount_amount numeric(20,2) NOT NULL DEFAULT 0 CHECK (discount_amount >= 0),
    subtotal numeric(20,2) NOT NULL CHECK (subtotal > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT order_adjustment_items_amount_check CHECK (gross_amount - discount_amount = subtotal)
);
CREATE INDEX order_adjustment_items_order_idx ON order_adjustment_items(store_id, order_id);
CREATE INDEX order_adjustment_items_adjustment_idx ON order_adjustment_items(adjustment_id);

UPDATE projects p
SET material_total = totals.amount
FROM (
    SELECT store_id, project_id, COALESCE(SUM(final_amount - returned_amount), 0) amount
    FROM orders
    WHERE project_id IS NOT NULL AND status <> 'REVERSED'
    GROUP BY store_id, project_id
) totals
WHERE p.store_id = totals.store_id AND p.id = totals.project_id;
