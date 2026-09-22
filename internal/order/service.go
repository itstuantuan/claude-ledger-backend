package order

import (
	"bytes"
	"cloud-ledger-backend/internal/platform/apperror"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

type Service struct {
	db  *gorm.DB
	now func() time.Time
}

func NewService(db *gorm.DB) *Service { return &Service{db: db, now: time.Now} }

type materialRow struct {
	ID                                       uuid.UUID
	Name, Brand, Specification, Unit, Status string
	DefaultSalePrice                         decimal.Decimal
}
type createMeta struct {
	StoreID, UserID               uuid.UUID
	Key, RequestID, IP, UserAgent string
}

func (s *Service) Create(ctx context.Context, meta createMeta, input Input) (Response, bool, error) {
	calc, err := Calculate(input)
	if err != nil {
		return Response{}, false, err
	}
	workerID, err := uuid.Parse(input.WorkerID)
	if err != nil {
		return Response{}, false, apperror.Validation(map[string][]string{"workerId": {"请选择油漆工"}})
	}
	payload, _ := json.Marshal(input)
	hash := sha256.Sum256(payload)
	var output Response
	replayed := false
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := s.now().UTC()
		record := IdempotencyRecord{ID: uuid.New(), StoreID: meta.StoreID, UserID: meta.UserID, IdempotencyKey: meta.Key, Operation: "CREATE_ORDER", RequestHash: hash[:], Status: "PROCESSING", CreatedAt: now, ExpiresAt: now.Add(24 * time.Hour)}
		insert := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&record)
		if insert.Error != nil {
			return insert.Error
		}
		if insert.RowsAffected == 0 {
			var old IdempotencyRecord
			if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("store_id=? AND idempotency_key=?", meta.StoreID, meta.Key).First(&old).Error; e != nil {
				return e
			}
			if old.Operation != "CREATE_ORDER" || !bytes.Equal(old.RequestHash, hash[:]) {
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
		var worker WorkerAccount
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("store_id=? AND id=?", meta.StoreID, workerID).First(&worker).Error; e != nil {
			if e == gorm.ErrRecordNotFound {
				return apperror.New(404, "WORKER_NOT_FOUND", "油漆工不存在。")
			}
			return e
		}
		if worker.Status != "ACTIVE" {
			return apperror.New(422, "WORKER_UNAVAILABLE", "油漆工已停用。")
		}
		var ledgerTotal decimal.Decimal
		if e := tx.Table("ledger_entries").Select("COALESCE(SUM(signed_amount),0)").Where("store_id=? AND worker_id=?", meta.StoreID, worker.ID).Scan(&ledgerTotal).Error; e != nil {
			return e
		}
		if !ledgerTotal.Equal(worker.CurrentReceivable) {
			return apperror.New(409, "ACCOUNT_INTEGRITY_ERROR", "客户应收汇总与账务流水不一致，请联系管理员。")
		}
		var timezone string
		if e := tx.Table("stores").Select("timezone").Where("id=?", meta.StoreID).Scan(&timezone).Error; e != nil {
			return e
		}
		occurred, businessDate, e := parseOccurred(input.OccurredAt, timezone)
		if e != nil {
			return apperror.Validation(map[string][]string{"occurredAt": {"业务日期格式不正确"}})
		}
		var projectID *uuid.UUID
		var projectName *string
		if input.ProjectID != nil && *input.ProjectID != "" {
			id, e := uuid.Parse(*input.ProjectID)
			if e != nil {
				return apperror.Validation(map[string][]string{"projectId": {"项目无效"}})
			}
			var p struct {
				ID           uuid.UUID
				Name, Status string
			}
			if e = tx.Table("projects").Where("store_id=? AND id=?", meta.StoreID, id).Scan(&p).Error; e != nil {
				return e
			}
			if p.ID == uuid.Nil {
				return apperror.New(404, "PROJECT_NOT_FOUND", "工地项目不存在。")
			}
			if p.Status == "CANCELLED" {
				return apperror.New(422, "PROJECT_UNAVAILABLE", "工地项目已取消。")
			}
			projectID = &id
			projectName = &p.Name
		} else if input.ProjectName != nil && strings.TrimSpace(*input.ProjectName) != "" {
			v := strings.TrimSpace(*input.ProjectName)
			if len([]rune(v)) > 80 {
				return apperror.Validation(map[string][]string{"projectName": {"工地名称不能超过 80 个字符"}})
			}
			projectName = &v
		}
		ids := make([]uuid.UUID, len(calc.Lines))
		for i, line := range calc.Lines {
			id, e := uuid.Parse(line.MaterialID)
			if e != nil {
				return apperror.Validation(map[string][]string{"items": {"材料无效"}})
			}
			ids[i] = id
		}
		var materials []materialRow
		if e := tx.Clauses(clause.Locking{Strength: "SHARE"}).Table("materials").Where("store_id=? AND id IN ?", meta.StoreID, ids).Order("id").Find(&materials).Error; e != nil {
			return e
		}
		byID := map[uuid.UUID]materialRow{}
		for _, m := range materials {
			byID[m.ID] = m
		}
		if len(byID) != len(ids) {
			return apperror.New(422, "MATERIAL_UNAVAILABLE", "所选材料不存在或已停用。")
		}
		orderID := uuid.New()
		orderNo, e := nextNumber(tx, meta.StoreID, businessDate, "YL")
		if e != nil {
			return e
		}
		items := make([]ItemModel, len(calc.Lines))
		itemResponses := make([]ItemResponse, len(calc.Lines))
		for i, line := range calc.Lines {
			m := byID[ids[i]]
			if m.Status != "ACTIVE" {
				return apperror.New(422, "MATERIAL_UNAVAILABLE", "所选材料不存在或已停用。")
			}
			var customer struct{ Price decimal.Decimal }
			priceQuery := tx.Table("customer_prices").Select("price").Where("store_id=? AND worker_id=? AND material_id=? AND status='ACTIVE' AND effective_from<=? AND (effective_to IS NULL OR effective_to>?)", meta.StoreID, worker.ID, m.ID, occurred, occurred).Order("effective_from DESC").Limit(1).Scan(&customer)
			if e = priceQuery.Error; e != nil {
				return e
			}
			source := "MANUAL"
			if priceQuery.RowsAffected > 0 && line.UnitPrice.Equal(customer.Price) {
				source = "CUSTOMER"
			} else if line.UnitPrice.Equal(m.DefaultSalePrice) {
				source = "DEFAULT"
			}
			items[i] = ItemModel{ID: uuid.New(), StoreID: meta.StoreID, OrderID: orderID, MaterialID: m.ID, MaterialNameSnapshot: m.Name, BrandSnapshot: m.Brand, SpecificationSnapshot: m.Specification, UnitSnapshot: m.Unit, UnitPrice: line.UnitPrice, Quantity: line.Quantity, GrossAmount: line.Gross, DiscountAmount: line.Discount, Subtotal: line.Subtotal, PriceSource: source, CreatedAt: now}
			itemResponses[i] = ItemResponse{MaterialID: m.ID.String(), MaterialName: m.Name, Specification: m.Specification, Unit: m.Unit, Quantity: line.Quantity.String(), UnitPrice: line.UnitPrice.StringFixed(2), Discount: line.Discount.StringFixed(2), Subtotal: line.Subtotal.StringFixed(2)}
		}
		if calc.Prepaid.IsPositive() {
			account := PrepaidAccount{ID: uuid.New(), StoreID: meta.StoreID, WorkerID: worker.ID, Balance: decimal.Zero, Version: 1, CreatedAt: now, UpdatedAt: now}
			if e = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&account).Error; e != nil {
				return e
			}
			if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("store_id=? AND worker_id=?", meta.StoreID, worker.ID).First(&account).Error; e != nil {
				return e
			}
			if account.Balance.LessThan(calc.Prepaid) {
				return apperror.New(422, "INSUFFICIENT_PREPAID_BALANCE", "预存余额不足。")
			}
			var prepaidTotal decimal.Decimal
			if e = tx.Table("prepaid_transactions").Select("COALESCE(SUM(signed_amount),0)").Where("store_id=? AND worker_id=?", meta.StoreID, worker.ID).Scan(&prepaidTotal).Error; e != nil {
				return e
			}
			if !account.Balance.Equal(worker.PrepaidBalance) || !account.Balance.Equal(prepaidTotal) {
				return apperror.New(409, "ACCOUNT_INTEGRITY_ERROR", "客户预存汇总与预存流水不一致，请联系管理员。")
			}
			before := account.Balance
			after := before.Sub(calc.Prepaid)
			if e = tx.Model(&PrepaidAccount{}).Where("id=?", account.ID).Updates(map[string]any{"balance": after, "version": gorm.Expr("version+1"), "updated_at": now}).Error; e != nil {
				return e
			}
			no, e := nextNumber(tx, meta.StoreID, businessDate, "LS")
			if e != nil {
				return e
			}
			pt := PrepaidTransaction{ID: uuid.New(), StoreID: meta.StoreID, WorkerID: worker.ID, TransactionNo: no, Type: "DEDUCTION", SignedAmount: calc.Prepaid.Neg(), BalanceBefore: before, BalanceAfter: after, SourceType: "ORDER", SourceID: orderID, Remark: strings.TrimSpace(input.Note), OccurredAt: occurred, CreatedBy: meta.UserID, CreatedAt: now}
			if e = tx.Create(&pt).Error; e != nil {
				return e
			}
		}
		status := "CONFIRMED"
		settled := calc.Payment.Add(calc.Prepaid)
		if calc.Receivable.IsZero() {
			status = "PAID"
		} else if settled.IsPositive() {
			status = "PARTIALLY_PAID"
		}
		order := Model{ID: orderID, StoreID: meta.StoreID, OrderNo: orderNo, WorkerID: worker.ID, WorkerNameSnapshot: worker.Name, ProjectID: projectID, ProjectNameSnapshot: projectName, Status: status, GoodsAmount: calc.Goods, DiscountAmount: calc.Discount, FinalAmount: calc.Final, PaymentAmount: calc.Payment, PrepaidDeductionAmount: calc.Prepaid, AddedReceivable: calc.Receivable, ReturnedAmount: decimal.Zero, SettledAmount: settled, OutstandingAmount: calc.Receivable, PaymentMethod: input.PaymentMethod, Remark: strings.TrimSpace(input.Note), OccurredAt: occurred, ConfirmedAt: now, CreatedBy: meta.UserID, CreatedAt: now, UpdatedAt: now}
		if e = tx.Create(&order).Error; e != nil {
			return e
		}
		if e = tx.Create(&items).Error; e != nil {
			return e
		}
		balance := worker.CurrentReceivable
		entries := []struct {
			kind   string
			amount decimal.Decimal
		}{{"ORDER_CHARGE", calc.Final}}
		if calc.Payment.IsPositive() {
			entries = append(entries, struct {
				kind   string
				amount decimal.Decimal
			}{"ORDER_PAYMENT", calc.Payment.Neg()})
		}
		if calc.Prepaid.IsPositive() {
			entries = append(entries, struct {
				kind   string
				amount decimal.Decimal
			}{"PREPAID_DEDUCTION", calc.Prepaid.Neg()})
		}
		for _, entry := range entries {
			balance = balance.Add(entry.amount)
			no, e := nextNumber(tx, meta.StoreID, businessDate, "LS")
			if e != nil {
				return e
			}
			ledger := LedgerEntry{ID: uuid.New(), StoreID: meta.StoreID, WorkerID: worker.ID, ProjectID: projectID, LedgerNo: no, Type: entry.kind, SignedAmount: entry.amount, BalanceAfter: balance, SourceType: "ORDER", SourceID: orderID, SourceNo: orderNo, Remark: order.Remark, OccurredAt: occurred, CreatedBy: meta.UserID, CreatedAt: now}
			if e = tx.Create(&ledger).Error; e != nil {
				return e
			}
		}
		if calc.Payment.IsPositive() {
			paymentNo, e := nextNumber(tx, meta.StoreID, businessDate, "SK")
			if e != nil {
				return e
			}
			paymentID := uuid.New()
			payment := Payment{ID: paymentID, StoreID: meta.StoreID, PaymentNo: paymentNo, WorkerID: worker.ID, OrderID: &orderID, Amount: calc.Payment, PaymentMethod: *input.PaymentMethod, OccurredAt: occurred, Status: "CONFIRMED", Remark: order.Remark, CreatedBy: meta.UserID, CreatedAt: now}
			if e = tx.Create(&payment).Error; e != nil {
				return e
			}
			financeNo, e := nextNumber(tx, meta.StoreID, businessDate, "LS")
			if e != nil {
				return e
			}
			wid := worker.ID
			finance := FinancialTransaction{ID: uuid.New(), StoreID: meta.StoreID, TransactionNo: financeNo, Type: "ORDER_PAYMENT", Direction: "INCOME", Amount: calc.Payment, PaymentMethod: *input.PaymentMethod, WorkerID: &wid, SourceType: "PAYMENT", SourceID: paymentID, SourceNo: paymentNo, OccurredAt: occurred, Remark: order.Remark, CreatedBy: meta.UserID, CreatedAt: now}
			if e = tx.Create(&finance).Error; e != nil {
				return e
			}
		}
		if e = tx.Model(&WorkerAccount{}).Where("store_id=? AND id=?", meta.StoreID, worker.ID).Updates(map[string]any{"material_total": gorm.Expr("material_total + ?", calc.Final), "payment_total": gorm.Expr("payment_total + ?", calc.Payment), "prepaid_balance": gorm.Expr("prepaid_balance - ?", calc.Prepaid), "current_receivable": balance, "last_transaction_at": occurred, "version": gorm.Expr("version+1"), "updated_at": now}).Error; e != nil {
			return e
		}
		if e = tx.Model(&struct{ ID uuid.UUID }{}).Table("materials").Where("store_id=? AND id IN ?", meta.StoreID, ids).UpdateColumn("sales_count", gorm.Expr("sales_count+1")).Error; e != nil {
			return e
		}
		var operator string
		if e = tx.Table("users").Select("name").Where("id=? AND store_id=?", meta.UserID, meta.StoreID).Scan(&operator).Error; e != nil {
			return e
		}
		output = responseFrom(order, itemResponses, operator)
		body, _ := json.Marshal(output)
		audit := AuditLog{ID: uuid.New(), StoreID: meta.StoreID, UserID: meta.UserID, Action: "order.create", ResourceType: "ORDER", ResourceID: orderID, AfterData: body, RequestID: meta.RequestID, IP: meta.IP, UserAgent: meta.UserAgent, CreatedAt: now}
		if e = tx.Create(&audit).Error; e != nil {
			return e
		}
		resource := "ORDER"
		statusCode := 201
		if e = tx.Model(&IdempotencyRecord{}).Where("id=?", record.ID).Updates(map[string]any{"resource_type": resource, "resource_id": orderID, "response_status": statusCode, "response_data": body, "status": "COMPLETED"}).Error; e != nil {
			return e
		}
		return nil
	})
	return output, replayed, err
}
func nextNumber(tx *gorm.DB, store uuid.UUID, date time.Time, prefix string) (string, error) {
	var value int64
	err := tx.Raw(`INSERT INTO business_sequences(store_id,business_date,business_type,last_value) VALUES(?,?,?,1) ON CONFLICT(store_id,business_date,business_type) DO UPDATE SET last_value=business_sequences.last_value+1,updated_at=now() RETURNING last_value`, store, date.Format("2006-01-02"), prefix).Scan(&value).Error
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%s%04d", prefix, date.Format("20060102"), value), nil
}
func parseOccurred(raw, zone string) (time.Time, time.Time, error) {
	location, err := time.LoadLocation(zone)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if day, err := time.ParseInLocation("2006-01-02", raw, location); err == nil {
		return day.UTC(), day, nil
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	local := value.In(location)
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
	return value.UTC(), day, nil
}
func responseFrom(o Model, items []ItemResponse, operator string) Response {
	var projectID *string
	if o.ProjectID != nil {
		v := o.ProjectID.String()
		projectID = &v
	}
	return Response{ID: o.ID.String(), OrderNo: o.OrderNo, WorkerID: o.WorkerID.String(), WorkerName: o.WorkerNameSnapshot, ProjectID: projectID, ProjectName: o.ProjectNameSnapshot, Items: items, GoodsAmount: o.GoodsAmount.StringFixed(2), DiscountAmount: o.DiscountAmount.StringFixed(2), FinalAmount: o.FinalAmount.StringFixed(2), PaymentAmount: o.PaymentAmount.StringFixed(2), PrepaidDeduction: o.PrepaidDeductionAmount.StringFixed(2), AddedReceivable: o.AddedReceivable.StringFixed(2), ReturnedAmount: o.ReturnedAmount.StringFixed(2), SettledAmount: o.SettledAmount.StringFixed(2), OutstandingAmount: o.OutstandingAmount.StringFixed(2), PaymentMethod: o.PaymentMethod, Status: o.Status, Note: o.Remark, OccurredAt: o.OccurredAt.Format(time.RFC3339), CreatedAt: o.CreatedAt.Format(time.RFC3339), OperatorName: operator}
}
