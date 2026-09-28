CREATE TABLE payment_allocations (
    id uuid PRIMARY KEY,
    store_id uuid NOT NULL REFERENCES stores(id) ON DELETE RESTRICT,
    payment_id uuid NOT NULL REFERENCES payments(id) ON DELETE RESTRICT,
    order_id uuid NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    amount numeric(20,2) NOT NULL CHECK (amount > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (payment_id, order_id)
);

CREATE INDEX payment_allocations_payment_idx ON payment_allocations(store_id, payment_id);
CREATE INDEX payment_allocations_order_idx ON payment_allocations(store_id, order_id);

INSERT INTO payment_allocations(id, store_id, payment_id, order_id, amount, created_at)
SELECT gen_random_uuid(), store_id, id, order_id, amount, created_at
FROM payments
WHERE order_id IS NOT NULL AND status = 'CONFIRMED';
