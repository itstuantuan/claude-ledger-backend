ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_balance_check;
UPDATE orders
SET final_amount = final_amount - returned_amount,
    goods_amount = final_amount - returned_amount + discount_amount,
    added_receivable = final_amount - returned_amount - payment_amount - prepaid_deduction_amount,
    returned_amount = 0;
ALTER TABLE orders ADD CONSTRAINT orders_settlement_check
    CHECK (final_amount = payment_amount + prepaid_deduction_amount + added_receivable);
ALTER TABLE orders DROP COLUMN IF EXISTS version;
DROP TABLE IF EXISTS order_adjustment_items;
DROP TABLE IF EXISTS order_adjustments;
