package order

import (
	"bytes"
	"cloud-ledger-backend/internal/platform/apperror"
	"context"
	"crypto/sha256"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

type materialBalance struct {
	MaterialID                                      uuid.UUID
	Name, Brand, Specification, Unit                string
	Quantity, GrossAmount, DiscountAmount, Subtotal decimal.Decimal
}

func (s *Service) Adjust(ctx context.Context, meta createMeta, orderID uuid.UUID, input AdjustmentInput) (AdjustmentResponse, bool, error) {
	if input.Type != "SUPPLEMENT" && input.Type != "RETURN" {
		return AdjustmentResponse{}, false, apperror.Validation(map[string][]string{"type": {"调整类型无效"}})
	}
	if input.ExpectedVersion < 1 {
		return AdjustmentResponse{}, false, apperror.Validation(map[string][]string{"expectedVersion": {"订单版本无效"}})
	}
	if len(input.Items) == 0 {
		return AdjustmentResponse{}, false, apperror.Validation(map[string][]string{"items": {"至少添加一种材料"}})
	}
	if len([]rune(strings.TrimSpace(input.Note))) > 200 {
		return AdjustmentResponse{}, false, apperror.Validation(map[string][]string{"note": {"备注不能超过 200 个字符"}})
	}
	payload, _ := json.Marshal(struct {
		OrderID uuid.UUID
		Input   AdjustmentInput
	}{orderID, input})
	hash := sha256.Sum256(payload)
	var output AdjustmentResponse
	replayed := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := s.now().UTC()
		record := IdempotencyRecord{ID: uuid.New(), StoreID: meta.StoreID, UserID: meta.UserID, IdempotencyKey: meta.Key, Operation: "ADJUST_ORDER", RequestHash: hash[:], Status: "PROCESSING", CreatedAt: now, ExpiresAt: now.Add(24 * time.Hour)}
		insert := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&record)
		if insert.Error != nil {
			return insert.Error
		}
		if insert.RowsAffected == 0 {
			var old IdempotencyRecord
			if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("store_id=? AND idempotency_key=?", meta.StoreID, meta.Key).First(&old).Error; e != nil {
				return e
			}
			if old.Operation != "ADJUST_ORDER" || !bytes.Equal(old.RequestHash, hash[:]) {
				return apperror.New(409, "IDEMPOTENCY_CONFLICT", "防重复提交标识已用于不同请求。")
			}
			if old.Status == "COMPLETED" && len(old.ResponseData) > 0 {
				if e := json.Unmarshal(old.ResponseData, &output); e != nil {
					return e
				}
				replayed = true
				return nil
			}
			return apperror.New(409, "IDEMPOTENCY_CONFLICT", "相同请求正在处理中，请稍后核对。")
		}

		var order Model
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("store_id=? AND id=?", meta.StoreID, orderID).First(&order).Error; e != nil {
			if e == gorm.ErrRecordNotFound {
				return apperror.New(404, "ORDER_NOT_FOUND", "用料单不存在。")
			}
			return e
		}
		if order.Status == "REVERSED" && input.Type == "RETURN" {
			return apperror.New(409, "ORDER_REVERSED", "已全部退清的用料单不能继续退料。")
		}
		if order.Version != input.ExpectedVersion {
			return apperror.New(409, "ORDER_VERSION_CONFLICT", "用料单已被其他人修改，请刷新后重试。")
		}
		var timezone string
		if e := tx.Table("stores").Select("timezone").Where("id=?", meta.StoreID).Scan(&timezone).Error; e != nil {
			return e
		}
		occurred, businessDate, e := parseOccurred(input.OccurredAt, timezone)
		if e != nil {
			return apperror.Validation(map[string][]string{"occurredAt": {"业务日期格式不正确"}})
		}
		if occurred.Before(order.OccurredAt) {
			return apperror.Validation(map[string][]string{"occurredAt": {"调整日期不能早于开单日期"}})
		}

		balances, e := currentBalances(tx, meta.StoreID, orderID)
		if e != nil {
			return e
		}
		seen := map[uuid.UUID]bool{}
		requestedIDs := make([]uuid.UUID, len(input.Items))
		for i, line := range input.Items {
			id, parseErr := uuid.Parse(line.MaterialID)
			if parseErr != nil || seen[id] {
				return apperror.Validation(map[string][]string{"items": {"材料无效或重复"}})
			}
			seen[id] = true
			requestedIDs[i] = id
		}
		var materials []materialRow
		if e := tx.Table("materials").Where("store_id=? AND id IN ?", meta.StoreID, requestedIDs).Find(&materials).Error; e != nil {
			return e
		}
		materialByID := map[uuid.UUID]materialRow{}
		for _, material := range materials {
			materialByID[material.ID] = material
		}
		if len(materialByID) != len(requestedIDs) {
			return apperror.New(422, "MATERIAL_UNAVAILABLE", "所选材料不存在。")
		}

		adjustmentID := uuid.New()
		itemModels := make([]AdjustmentItemModel, len(input.Items))
		itemResponses := make([]ItemResponse, len(input.Items))
		goods, discount, final := decimal.Zero, decimal.Zero, decimal.Zero
		for i, line := range input.Items {
			quantity, parseErr := parseQuantity(line.Quantity)
			if parseErr != nil || !quantity.IsPositive() {
				return apperror.Validation(map[string][]string{"items": {"数量格式不正确"}})
			}
			material := materialByID[requestedIDs[i]]
			materialName, materialBrand, materialSpecification, materialUnit := material.Name, material.Brand, material.Specification, material.Unit
			var unitPrice, lineDiscount, gross, subtotal decimal.Decimal
			if input.Type == "SUPPLEMENT" {
				if material.Status != "ACTIVE" {
					return apperror.New(422, "MATERIAL_UNAVAILABLE", "补料只能选择启用的材料。")
				}
				unitPrice, parseErr = parseMoney(line.UnitPrice)
				if parseErr != nil {
					return apperror.Validation(map[string][]string{"items": {"单价格式不正确"}})
				}
				lineDiscount, parseErr = parseMoney(line.Discount)
				if parseErr != nil {
					return apperror.Validation(map[string][]string{"items": {"优惠格式不正确"}})
				}
				gross = unitPrice.Mul(quantity).Round(2)
				if lineDiscount.GreaterThanOrEqual(gross) {
					return apperror.New(422, "ADJUSTMENT_AMOUNT_ZERO", "补料金额必须大于零。")
				}
				subtotal = gross.Sub(lineDiscount)
			} else {
				balance, ok := balances[requestedIDs[i]]
				if !ok || quantity.GreaterThan(balance.Quantity) {
					return apperror.New(422, "RETURN_QUANTITY_EXCEEDED", "退料数量超过当前可退数量。")
				}
				unitPrice = balance.GrossAmount.Div(balance.Quantity).Round(2)
				gross = unitPrice.Mul(quantity).Round(2)
				subtotal = balance.Subtotal.Mul(quantity).Div(balance.Quantity).Round(2)
				lineDiscount = gross.Sub(subtotal)
				if lineDiscount.IsNegative() {
					lineDiscount = decimal.Zero
					subtotal = gross
				}
				if !subtotal.IsPositive() {
					return apperror.New(422, "ADJUSTMENT_AMOUNT_ZERO", "退料金额必须大于零。")
				}
				materialName, materialBrand, materialSpecification, materialUnit = balance.Name, balance.Brand, balance.Specification, balance.Unit
			}
			goods = goods.Add(gross)
			discount = discount.Add(lineDiscount)
			final = final.Add(subtotal)
			itemModels[i] = AdjustmentItemModel{ID: uuid.New(), StoreID: meta.StoreID, AdjustmentID: adjustmentID, OrderID: orderID, MaterialID: material.ID, MaterialNameSnapshot: materialName, BrandSnapshot: materialBrand, SpecificationSnapshot: materialSpecification, UnitSnapshot: materialUnit, UnitPrice: unitPrice, Quantity: quantity, GrossAmount: gross, DiscountAmount: lineDiscount, Subtotal: subtotal, CreatedAt: now}
			itemResponses[i] = ItemResponse{MaterialID: material.ID.String(), MaterialName: materialName, Specification: materialSpecification, Unit: materialUnit, Quantity: quantity.String(), UnitPrice: unitPrice.StringFixed(2), Discount: lineDiscount.StringFixed(2), Subtotal: subtotal.StringFixed(2)}
		}
		if input.Type == "RETURN" && final.GreaterThan(order.OutstandingAmount) {
			return apperror.New(422, "RETURN_EXCEEDS_OUTSTANDING", "退料金额超过当前未结金额，请先处理已收款。")
		}
		prefix := "BL"
		if input.Type == "RETURN" {
			prefix = "TL"
		}
		adjustmentNo, e := nextNumber(tx, meta.StoreID, businessDate, prefix)
		if e != nil {
			return e
		}
		adjustment := AdjustmentModel{ID: adjustmentID, StoreID: meta.StoreID, OrderID: orderID, AdjustmentNo: adjustmentNo, Type: input.Type, GoodsAmount: goods, DiscountAmount: discount, FinalAmount: final, Remark: strings.TrimSpace(input.Note), OccurredAt: occurred, CreatedBy: meta.UserID, CreatedAt: now}
		if e := tx.Create(&adjustment).Error; e != nil {
			return e
		}
		if e := tx.Create(&itemModels).Error; e != nil {
			return e
		}

		delta := final
		ledgerType := "ADJUSTMENT"
		orderUpdates := map[string]any{"goods_amount": gorm.Expr("goods_amount + ?", goods), "discount_amount": gorm.Expr("discount_amount + ?", discount), "final_amount": gorm.Expr("final_amount + ?", final), "added_receivable": gorm.Expr("added_receivable + ?", final), "outstanding_amount": gorm.Expr("outstanding_amount + ?", final)}
		workerUpdates := map[string]any{"material_total": gorm.Expr("material_total + ?", final)}
		if input.Type == "RETURN" {
			delta = final.Neg()
			ledgerType = "RETURN_CREDIT"
			orderUpdates = map[string]any{"returned_amount": gorm.Expr("returned_amount + ?", final), "added_receivable": gorm.Expr("added_receivable - ?", final), "outstanding_amount": gorm.Expr("outstanding_amount - ?", final)}
			workerUpdates = map[string]any{"return_total": gorm.Expr("return_total + ?", final)}
		}
		newOutstanding := order.OutstandingAmount.Add(delta)
		status := "CONFIRMED"
		newNetAmount := order.FinalAmount.Sub(order.ReturnedAmount).Add(delta)
		if newNetAmount.IsZero() {
			status = "REVERSED"
		} else if newOutstanding.IsZero() {
			status = "PAID"
		} else if order.SettledAmount.IsPositive() {
			status = "PARTIALLY_PAID"
		}
		orderUpdates["status"] = status
		orderUpdates["version"] = gorm.Expr("version + 1")
		orderUpdates["updated_at"] = now
		result := tx.Model(&Model{}).Where("store_id=? AND id=? AND version=?", meta.StoreID, orderID, input.ExpectedVersion).Updates(orderUpdates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return apperror.New(409, "ORDER_VERSION_CONFLICT", "用料单已被其他人修改，请刷新后重试。")
		}

		var ledgerTotal decimal.Decimal
		if e := tx.Table("ledger_entries").Select("COALESCE(SUM(signed_amount),0)").Where("store_id=? AND worker_id=?", meta.StoreID, order.WorkerID).Scan(&ledgerTotal).Error; e != nil {
			return e
		}
		var worker WorkerAccount
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("store_id=? AND id=?", meta.StoreID, order.WorkerID).First(&worker).Error; e != nil {
			return e
		}
		if !ledgerTotal.Equal(worker.CurrentReceivable) {
			return apperror.New(409, "ACCOUNT_INTEGRITY_ERROR", "客户应收汇总与账务流水不一致，请联系管理员。")
		}
		newBalance := worker.CurrentReceivable.Add(delta)
		workerUpdates["current_receivable"] = newBalance
		workerUpdates["last_transaction_at"] = occurred
		workerUpdates["version"] = gorm.Expr("version + 1")
		workerUpdates["updated_at"] = now
		if e := tx.Model(&WorkerAccount{}).Where("store_id=? AND id=?", meta.StoreID, order.WorkerID).Updates(workerUpdates).Error; e != nil {
			return e
		}
		if order.ProjectID != nil {
			if e := tx.Table("projects").Where("store_id=? AND id=?", meta.StoreID, *order.ProjectID).Updates(map[string]any{"material_total": gorm.Expr("material_total + ?", delta), "version": gorm.Expr("version + 1"), "updated_at": now}).Error; e != nil {
				return e
			}
		}
		ledgerNo, e := nextNumber(tx, meta.StoreID, businessDate, "LS")
		if e != nil {
			return e
		}
		ledger := LedgerEntry{ID: uuid.New(), StoreID: meta.StoreID, WorkerID: order.WorkerID, ProjectID: order.ProjectID, LedgerNo: ledgerNo, Type: ledgerType, SignedAmount: delta, BalanceAfter: newBalance, SourceType: "ORDER_ADJUSTMENT", SourceID: adjustmentID, SourceNo: adjustmentNo, Remark: adjustment.Remark, OccurredAt: occurred, CreatedBy: meta.UserID, CreatedAt: now}
		if e := tx.Create(&ledger).Error; e != nil {
			return e
		}
		if input.Type == "SUPPLEMENT" {
			if e := tx.Table("materials").Where("store_id=? AND id IN ?", meta.StoreID, requestedIDs).UpdateColumn("sales_count", gorm.Expr("sales_count + 1")).Error; e != nil {
				return e
			}
		}
		var operator string
		if e := tx.Table("users").Select("name").Where("store_id=? AND id=?", meta.StoreID, meta.UserID).Scan(&operator).Error; e != nil {
			return e
		}
		output = adjustmentResponse(adjustment, itemResponses, operator)
		body, _ := json.Marshal(output)
		audit := AuditLog{ID: uuid.New(), StoreID: meta.StoreID, UserID: meta.UserID, Action: "order.adjust." + strings.ToLower(input.Type), ResourceType: "ORDER", ResourceID: orderID, AfterData: body, RequestID: meta.RequestID, IP: meta.IP, UserAgent: meta.UserAgent, CreatedAt: now}
		if e := tx.Create(&audit).Error; e != nil {
			return e
		}
		if e := tx.Model(&IdempotencyRecord{}).Where("id=?", record.ID).Updates(map[string]any{"resource_type": "ORDER_ADJUSTMENT", "resource_id": adjustmentID, "response_status": 201, "response_data": body, "status": "COMPLETED"}).Error; e != nil {
			return e
		}
		return nil
	})
	return output, replayed, err
}

func (s *Service) ListAdjustments(ctx context.Context, store, orderID uuid.UUID) ([]AdjustmentResponse, error) {
	var exists int64
	if e := s.db.WithContext(ctx).Table("orders").Where("store_id=? AND id=?", store, orderID).Count(&exists).Error; e != nil {
		return nil, e
	}
	if exists == 0 {
		return nil, apperror.New(404, "ORDER_NOT_FOUND", "用料单不存在。")
	}
	type row struct {
		AdjustmentModel
		OperatorName string
	}
	var rows []row
	if e := s.db.WithContext(ctx).Table("order_adjustments a").Select("a.*,u.name AS operator_name").Joins("JOIN users u ON u.id=a.created_by AND u.store_id=a.store_id").Where("a.store_id=? AND a.order_id=?", store, orderID).Order("a.occurred_at,a.created_at,a.id").Scan(&rows).Error; e != nil {
		return nil, e
	}
	ids := make([]uuid.UUID, len(rows))
	for i := range rows {
		ids[i] = rows[i].ID
	}
	var itemRows []AdjustmentItemModel
	if len(ids) > 0 {
		if e := s.db.WithContext(ctx).Where("store_id=? AND adjustment_id IN ?", store, ids).Order("created_at,id").Find(&itemRows).Error; e != nil {
			return nil, e
		}
	}
	byAdjustment := map[uuid.UUID][]ItemResponse{}
	for _, item := range itemRows {
		byAdjustment[item.AdjustmentID] = append(byAdjustment[item.AdjustmentID], itemResponseFromAdjustment(item))
	}
	result := make([]AdjustmentResponse, len(rows))
	for i, item := range rows {
		result[i] = adjustmentResponse(item.AdjustmentModel, byAdjustment[item.ID], item.OperatorName)
		if result[i].Items == nil {
			result[i].Items = []ItemResponse{}
		}
	}
	return result, nil
}

func currentBalances(db *gorm.DB, store, orderID uuid.UUID) (map[uuid.UUID]materialBalance, error) {
	all, err := currentBalancesForOrders(db, store, []uuid.UUID{orderID})
	return all[orderID], err
}

func currentBalancesForOrders(db *gorm.DB, store uuid.UUID, orderIDs []uuid.UUID) (map[uuid.UUID]map[uuid.UUID]materialBalance, error) {
	result := map[uuid.UUID]map[uuid.UUID]materialBalance{}
	if len(orderIDs) == 0 {
		return result, nil
	}
	var originals []ItemModel
	if e := db.Where("store_id=? AND order_id IN ?", store, orderIDs).Order("created_at,id").Find(&originals).Error; e != nil {
		return nil, e
	}
	var adjustments []struct {
		AdjustmentItemModel
		Type string
	}
	if e := db.Table("order_adjustment_items ai").Select("ai.*,a.type").Joins("JOIN order_adjustments a ON a.id=ai.adjustment_id AND a.store_id=ai.store_id").Where("ai.store_id=? AND ai.order_id IN ?", store, orderIDs).Order("a.occurred_at,a.created_at,ai.id").Scan(&adjustments).Error; e != nil {
		return nil, e
	}
	for _, item := range originals {
		if result[item.OrderID] == nil {
			result[item.OrderID] = map[uuid.UUID]materialBalance{}
		}
		balance := result[item.OrderID][item.MaterialID]
		balance.MaterialID, balance.Name, balance.Brand, balance.Specification, balance.Unit = item.MaterialID, item.MaterialNameSnapshot, item.BrandSnapshot, item.SpecificationSnapshot, item.UnitSnapshot
		balance.Quantity = balance.Quantity.Add(item.Quantity)
		balance.GrossAmount = balance.GrossAmount.Add(item.GrossAmount)
		balance.DiscountAmount = balance.DiscountAmount.Add(item.DiscountAmount)
		balance.Subtotal = balance.Subtotal.Add(item.Subtotal)
		result[item.OrderID][item.MaterialID] = balance
	}
	for _, row := range adjustments {
		item := row.AdjustmentItemModel
		if result[item.OrderID] == nil {
			result[item.OrderID] = map[uuid.UUID]materialBalance{}
		}
		sign := decimal.NewFromInt(1)
		if row.Type == "RETURN" {
			sign = sign.Neg()
		}
		balance := result[item.OrderID][item.MaterialID]
		balance.MaterialID, balance.Name, balance.Brand, balance.Specification, balance.Unit = item.MaterialID, item.MaterialNameSnapshot, item.BrandSnapshot, item.SpecificationSnapshot, item.UnitSnapshot
		balance.Quantity = balance.Quantity.Add(item.Quantity.Mul(sign))
		balance.GrossAmount = balance.GrossAmount.Add(item.GrossAmount.Mul(sign))
		balance.DiscountAmount = balance.DiscountAmount.Add(item.DiscountAmount.Mul(sign))
		balance.Subtotal = balance.Subtotal.Add(item.Subtotal.Mul(sign))
		result[item.OrderID][item.MaterialID] = balance
	}
	return result, nil
}

func adjustmentResponse(item AdjustmentModel, items []ItemResponse, operator string) AdjustmentResponse {
	return AdjustmentResponse{ID: item.ID.String(), AdjustmentNo: item.AdjustmentNo, Type: item.Type, Items: items, GoodsAmount: item.GoodsAmount.StringFixed(2), Discount: item.DiscountAmount.StringFixed(2), FinalAmount: item.FinalAmount.StringFixed(2), Note: item.Remark, OccurredAt: item.OccurredAt.Format(time.RFC3339), CreatedAt: item.CreatedAt.Format(time.RFC3339), OperatorName: operator}
}

func itemResponseFromAdjustment(item AdjustmentItemModel) ItemResponse {
	return ItemResponse{MaterialID: item.MaterialID.String(), MaterialName: item.MaterialNameSnapshot, Specification: item.SpecificationSnapshot, Unit: item.UnitSnapshot, Quantity: item.Quantity.String(), UnitPrice: item.UnitPrice.StringFixed(2), Discount: item.DiscountAmount.StringFixed(2), Subtotal: item.Subtotal.StringFixed(2)}
}
