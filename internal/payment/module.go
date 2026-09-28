package payment

import (
	"bytes"
	"cloud-ledger-backend/internal/auth"
	"cloud-ledger-backend/internal/platform/apperror"
	"cloud-ledger-backend/internal/platform/pagination"
	platformrequest "cloud-ledger-backend/internal/platform/request"
	"cloud-ledger-backend/internal/platform/response"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var validMethods = map[string]bool{"WECHAT": true, "ALIPAY": true, "CASH": true, "BANK_CARD": true, "OTHER": true}

type Input struct {
	WorkerID      string            `json:"workerId"`
	Amount        string            `json:"amount"`
	PaymentMethod string            `json:"paymentMethod"`
	OccurredAt    string            `json:"occurredAt"`
	Note          string            `json:"note"`
	Allocations   []AllocationInput `json:"allocations,omitempty"`
}

type AllocationInput struct {
	OrderID string `json:"orderId"`
	Amount  string `json:"amount"`
}

type AllocationResponse struct {
	OrderID string `json:"orderId"`
	OrderNo string `json:"orderNo"`
	Amount  string `json:"amount"`
}

type Response struct {
	ID                string               `json:"id"`
	TransactionNo     string               `json:"transactionNo"`
	WorkerID          string               `json:"workerId"`
	WorkerName        string               `json:"workerName"`
	Amount            string               `json:"amount"`
	PaymentMethod     string               `json:"paymentMethod"`
	OccurredAt        string               `json:"occurredAt"`
	Note              string               `json:"note"`
	OperatorName      string               `json:"operatorName"`
	CreatedAt         string               `json:"createdAt"`
	ReceivableBefore  string               `json:"receivableBefore"`
	ReceivableAfter   string               `json:"receivableAfter"`
	UnallocatedAmount string               `json:"unallocatedAmount"`
	Allocations       []AllocationResponse `json:"allocations"`
}

type paymentModel struct {
	ID, StoreID    uuid.UUID
	PaymentNo      string
	WorkerID       uuid.UUID
	OrderID        *uuid.UUID
	Amount         decimal.Decimal
	PaymentMethod  string
	OccurredAt     time.Time
	Status, Remark string
	CreatedBy      uuid.UUID
	CreatedAt      time.Time
}

func (paymentModel) TableName() string { return "payments" }

type allocationModel struct {
	ID, StoreID, PaymentID, OrderID uuid.UUID
	Amount                          decimal.Decimal
	CreatedAt                       time.Time
}

func (allocationModel) TableName() string { return "payment_allocations" }

type workerModel struct {
	ID, StoreID                                                    uuid.UUID
	Name, Status                                                   string
	MaterialTotal, PaymentTotal, PrepaidBalance, CurrentReceivable decimal.Decimal
	LastTransactionAt                                              *time.Time
	Version                                                        int64
}

func (workerModel) TableName() string { return "workers" }

type orderModel struct {
	ID, StoreID                      uuid.UUID
	OrderNo                          string
	WorkerID                         uuid.UUID
	Status                           string
	SettledAmount, OutstandingAmount decimal.Decimal
	OccurredAt, CreatedAt, UpdatedAt time.Time
	Version                          int64
}

func (orderModel) TableName() string { return "orders" }

type ledgerModel struct {
	ID, StoreID, WorkerID      uuid.UUID
	ProjectID                  *uuid.UUID
	LedgerNo, Type             string
	SignedAmount, BalanceAfter decimal.Decimal
	SourceType                 string
	SourceID                   uuid.UUID
	SourceNo, Remark           string
	OccurredAt                 time.Time
	CreatedBy                  uuid.UUID
	CreatedAt                  time.Time
}

func (ledgerModel) TableName() string { return "ledger_entries" }

type financialModel struct {
	ID, StoreID                    uuid.UUID
	TransactionNo, Type, Direction string
	Amount                         decimal.Decimal
	PaymentMethod                  string
	WorkerID                       *uuid.UUID
	SourceType                     string
	SourceID                       uuid.UUID
	SourceNo, Remark               string
	OccurredAt                     time.Time
	CreatedBy                      uuid.UUID
	CreatedAt                      time.Time
}

func (financialModel) TableName() string { return "financial_transactions" }

type idempotencyModel struct {
	ID, StoreID, UserID       uuid.UUID
	IdempotencyKey, Operation string
	RequestHash               []byte
	ResourceType              *string
	ResourceID                *uuid.UUID
	ResponseStatus            *int
	ResponseData              []byte `gorm:"type:jsonb"`
	Status                    string
	CreatedAt, ExpiresAt      time.Time
}

func (idempotencyModel) TableName() string { return "idempotency_records" }

type auditModel struct {
	ID, StoreID, UserID      uuid.UUID
	Action, ResourceType     string
	ResourceID               uuid.UUID
	AfterData                []byte `gorm:"type:jsonb"`
	RequestID, IP, UserAgent string
	CreatedAt                time.Time
}

func (auditModel) TableName() string { return "audit_logs" }

type createMeta struct {
	StoreID, UserID               uuid.UUID
	Key, RequestID, IP, UserAgent string
}

type Service struct {
	db  *gorm.DB
	now func() time.Time
}

func NewService(db *gorm.DB) *Service { return &Service{db: db, now: time.Now} }

func parseInput(input Input) (uuid.UUID, decimal.Decimal, error) {
	workerID, err := uuid.Parse(input.WorkerID)
	if err != nil {
		return uuid.Nil, decimal.Zero, apperror.Validation(map[string][]string{"workerId": {"请选择油漆工"}})
	}
	amount, err := decimal.NewFromString(input.Amount)
	if err != nil || !amount.IsPositive() || amount.Exponent() < -2 {
		return uuid.Nil, decimal.Zero, apperror.Validation(map[string][]string{"amount": {"金额格式不正确"}})
	}
	if !validMethods[input.PaymentMethod] {
		return uuid.Nil, decimal.Zero, apperror.Validation(map[string][]string{"paymentMethod": {"付款方式无效"}})
	}
	if len([]rune(strings.TrimSpace(input.Note))) > 200 {
		return uuid.Nil, decimal.Zero, apperror.Validation(map[string][]string{"note": {"备注不能超过 200 个字符"}})
	}
	return workerID, amount, nil
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

func nextNumber(tx *gorm.DB, store uuid.UUID, date time.Time, prefix string) (string, error) {
	var value int64
	err := tx.Raw(`INSERT INTO business_sequences(store_id,business_date,business_type,last_value) VALUES(?,?,?,1) ON CONFLICT(store_id,business_date,business_type) DO UPDATE SET last_value=business_sequences.last_value+1,updated_at=now() RETURNING last_value`, store, date.Format("2006-01-02"), prefix).Scan(&value).Error
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%s%04d", prefix, date.Format("20060102"), value), nil
}

type plannedAllocation struct {
	OrderID uuid.UUID
	OrderNo string
	Amount  decimal.Decimal
}

func planAllocations(orders []orderModel, amount decimal.Decimal) ([]plannedAllocation, decimal.Decimal) {
	remaining := amount
	result := make([]plannedAllocation, 0, len(orders))
	for _, order := range orders {
		if !remaining.IsPositive() {
			break
		}
		allocated := order.OutstandingAmount
		if allocated.GreaterThan(remaining) {
			allocated = remaining
		}
		if allocated.IsPositive() {
			result = append(result, plannedAllocation{OrderID: order.ID, OrderNo: order.OrderNo, Amount: allocated})
			remaining = remaining.Sub(allocated)
		}
	}
	return result, remaining
}

func planSpecifiedAllocations(orders []orderModel, requested []AllocationInput, paymentAmount decimal.Decimal) ([]plannedAllocation, decimal.Decimal, error) {
	byID := make(map[uuid.UUID]orderModel, len(orders))
	for _, order := range orders {
		byID[order.ID] = order
	}
	seen := map[uuid.UUID]bool{}
	total := decimal.Zero
	result := make([]plannedAllocation, 0, len(requested))
	for _, item := range requested {
		orderID, err := uuid.Parse(item.OrderID)
		if err != nil || seen[orderID] {
			return nil, decimal.Zero, apperror.Validation(map[string][]string{"allocations": {"指定订单无效或重复"}})
		}
		seen[orderID] = true
		amount, err := decimal.NewFromString(item.Amount)
		if err != nil || !amount.IsPositive() || amount.Exponent() < -2 {
			return nil, decimal.Zero, apperror.Validation(map[string][]string{"allocations": {"核销金额格式不正确"}})
		}
		order, ok := byID[orderID]
		if !ok {
			return nil, decimal.Zero, apperror.New(422, "PAYMENT_ORDER_UNAVAILABLE", "指定订单不存在、已结清或不属于该客户。")
		}
		if amount.GreaterThan(order.OutstandingAmount) {
			return nil, decimal.Zero, apperror.New(422, "PAYMENT_ALLOCATION_EXCEEDED", "核销金额不能超过指定订单的未结金额。")
		}
		total = total.Add(amount)
		result = append(result, plannedAllocation{OrderID: orderID, OrderNo: order.OrderNo, Amount: amount})
	}
	if !total.Equal(paymentAmount) {
		return nil, decimal.Zero, apperror.New(422, "PAYMENT_ALLOCATION_MISMATCH", "指定订单的核销金额合计必须等于本次收款金额。")
	}
	return result, decimal.Zero, nil
}

func (s *Service) Create(ctx context.Context, meta createMeta, input Input) (Response, bool, error) {
	workerID, amount, err := parseInput(input)
	if err != nil {
		return Response{}, false, err
	}
	payload, _ := json.Marshal(input)
	hash := sha256.Sum256(payload)
	var output Response
	replayed := false
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := s.now().UTC()
		record := idempotencyModel{ID: uuid.New(), StoreID: meta.StoreID, UserID: meta.UserID, IdempotencyKey: meta.Key, Operation: "CREATE_PAYMENT", RequestHash: hash[:], Status: "PROCESSING", CreatedAt: now, ExpiresAt: now.Add(24 * time.Hour)}
		insert := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&record)
		if insert.Error != nil {
			return insert.Error
		}
		if insert.RowsAffected == 0 {
			var old idempotencyModel
			if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("store_id=? AND idempotency_key=?", meta.StoreID, meta.Key).First(&old).Error; e != nil {
				return e
			}
			if old.Operation != "CREATE_PAYMENT" || !bytes.Equal(old.RequestHash, hash[:]) {
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

		var worker workerModel
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("store_id=? AND id=?", meta.StoreID, workerID).First(&worker).Error; e != nil {
			if e == gorm.ErrRecordNotFound {
				return apperror.New(404, "WORKER_NOT_FOUND", "油漆工不存在。")
			}
			return e
		}
		if worker.Status != "ACTIVE" {
			return apperror.New(422, "WORKER_DISABLED", "已停用的油漆工不能登记收款。")
		}
		var ledgerTotal decimal.Decimal
		if e := tx.Table("ledger_entries").Select("COALESCE(SUM(signed_amount),0)").Where("store_id=? AND worker_id=?", meta.StoreID, workerID).Scan(&ledgerTotal).Error; e != nil {
			return e
		}
		if !ledgerTotal.Equal(worker.CurrentReceivable) {
			return apperror.New(409, "ACCOUNT_INTEGRITY_ERROR", "客户应收汇总与账务流水不一致，请联系管理员。")
		}
		if amount.GreaterThan(worker.CurrentReceivable) {
			return apperror.New(422, "PAYMENT_EXCEEDED", "收款金额不能超过客户当前应收。")
		}

		var timezone, operatorName string
		if e := tx.Table("stores").Select("timezone").Where("id=?", meta.StoreID).Scan(&timezone).Error; e != nil {
			return e
		}
		if e := tx.Table("users").Select("name").Where("store_id=? AND id=?", meta.StoreID, meta.UserID).Scan(&operatorName).Error; e != nil {
			return e
		}
		occurred, businessDate, e := parseOccurred(input.OccurredAt, timezone)
		if e != nil {
			return apperror.Validation(map[string][]string{"occurredAt": {"收款时间格式不正确"}})
		}

		var orders []orderModel
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("store_id=? AND worker_id=? AND outstanding_amount>0", meta.StoreID, workerID).Order("occurred_at,created_at,id").Find(&orders).Error; e != nil {
			return e
		}
		plans, unallocated := planAllocations(orders, amount)
		if len(input.Allocations) > 0 {
			plans, unallocated, e = planSpecifiedAllocations(orders, input.Allocations, amount)
			if e != nil {
				return e
			}
		}
		paymentNo, e := nextNumber(tx, meta.StoreID, businessDate, "SK")
		if e != nil {
			return e
		}
		paymentID := uuid.New()
		payment := paymentModel{ID: paymentID, StoreID: meta.StoreID, PaymentNo: paymentNo, WorkerID: workerID, Amount: amount, PaymentMethod: input.PaymentMethod, OccurredAt: occurred, Status: "CONFIRMED", Remark: strings.TrimSpace(input.Note), CreatedBy: meta.UserID, CreatedAt: now}
		if e := tx.Create(&payment).Error; e != nil {
			return e
		}
		allocations := make([]AllocationResponse, 0, len(plans))
		for _, plan := range plans {
			allocation := allocationModel{ID: uuid.New(), StoreID: meta.StoreID, PaymentID: paymentID, OrderID: plan.OrderID, Amount: plan.Amount, CreatedAt: now}
			if e := tx.Create(&allocation).Error; e != nil {
				return e
			}
			var order orderModel
			for _, candidate := range orders {
				if candidate.ID == plan.OrderID {
					order = candidate
					break
				}
			}
			outstanding := order.OutstandingAmount.Sub(plan.Amount)
			status := "PARTIALLY_PAID"
			if outstanding.IsZero() {
				status = "PAID"
			}
			result := tx.Model(&orderModel{}).Where("store_id=? AND id=? AND version=?", meta.StoreID, order.ID, order.Version).Updates(map[string]any{"settled_amount": gorm.Expr("settled_amount + ?", plan.Amount), "outstanding_amount": outstanding, "status": status, "version": gorm.Expr("version + 1"), "updated_at": now})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return apperror.New(409, "ORDER_VERSION_CONFLICT", "订单已被其他操作修改，请重新提交收款。")
			}
			allocations = append(allocations, AllocationResponse{OrderID: plan.OrderID.String(), OrderNo: plan.OrderNo, Amount: plan.Amount.StringFixed(2)})
		}

		newBalance := worker.CurrentReceivable.Sub(amount)
		if e := tx.Model(&workerModel{}).Where("store_id=? AND id=?", meta.StoreID, workerID).Updates(map[string]any{"payment_total": gorm.Expr("payment_total + ?", amount), "current_receivable": newBalance, "last_transaction_at": occurred, "version": gorm.Expr("version + 1"), "updated_at": now}).Error; e != nil {
			return e
		}
		ledgerNo, e := nextNumber(tx, meta.StoreID, businessDate, "LS")
		if e != nil {
			return e
		}
		ledger := ledgerModel{ID: uuid.New(), StoreID: meta.StoreID, WorkerID: workerID, LedgerNo: ledgerNo, Type: "PAYMENT", SignedAmount: amount.Neg(), BalanceAfter: newBalance, SourceType: "PAYMENT", SourceID: paymentID, SourceNo: paymentNo, Remark: payment.Remark, OccurredAt: occurred, CreatedBy: meta.UserID, CreatedAt: now}
		if e := tx.Create(&ledger).Error; e != nil {
			return e
		}
		financeNo, e := nextNumber(tx, meta.StoreID, businessDate, "LS")
		if e != nil {
			return e
		}
		wid := workerID
		finance := financialModel{ID: uuid.New(), StoreID: meta.StoreID, TransactionNo: financeNo, Type: "PAYMENT", Direction: "INCOME", Amount: amount, PaymentMethod: input.PaymentMethod, WorkerID: &wid, SourceType: "PAYMENT", SourceID: paymentID, SourceNo: paymentNo, Remark: payment.Remark, OccurredAt: occurred, CreatedBy: meta.UserID, CreatedAt: now}
		if e := tx.Create(&finance).Error; e != nil {
			return e
		}

		output = Response{ID: paymentID.String(), TransactionNo: paymentNo, WorkerID: workerID.String(), WorkerName: worker.Name, Amount: amount.StringFixed(2), PaymentMethod: input.PaymentMethod, OccurredAt: occurred.Format(time.RFC3339), Note: payment.Remark, OperatorName: operatorName, CreatedAt: now.Format(time.RFC3339), ReceivableBefore: worker.CurrentReceivable.StringFixed(2), ReceivableAfter: newBalance.StringFixed(2), UnallocatedAmount: unallocated.StringFixed(2), Allocations: allocations}
		body, _ := json.Marshal(output)
		audit := auditModel{ID: uuid.New(), StoreID: meta.StoreID, UserID: meta.UserID, Action: "payment.create", ResourceType: "PAYMENT", ResourceID: paymentID, AfterData: body, RequestID: meta.RequestID, IP: meta.IP, UserAgent: meta.UserAgent, CreatedAt: now}
		if e := tx.Create(&audit).Error; e != nil {
			return e
		}
		resource, statusCode := "PAYMENT", http.StatusCreated
		if e := tx.Model(&idempotencyModel{}).Where("id=?", record.ID).Updates(map[string]any{"resource_type": resource, "resource_id": paymentID, "response_status": statusCode, "response_data": body, "status": "COMPLETED"}).Error; e != nil {
			return e
		}
		return nil
	})
	return output, replayed, err
}

type queryRow struct {
	paymentModel
	WorkerName, OperatorName string
	ReceivableAfter          decimal.Decimal
}

func (s *Service) List(ctx context.Context, store uuid.UUID, q map[string]string, p pagination.Params) ([]Response, int64, error) {
	base := s.db.WithContext(ctx).Table("payments p").Where("p.store_id=? AND p.status='CONFIRMED'", store)
	if v := q["workerId"]; v != "" {
		base = base.Where("p.worker_id=?", v)
	}
	if v := q["search"]; v != "" {
		like := "%" + v + "%"
		base = base.Joins("JOIN workers sw ON sw.id=p.worker_id AND sw.store_id=p.store_id").Where("p.payment_no ILIKE ? OR sw.name ILIKE ?", like, like)
	}
	if v := q["from"]; v != "" {
		base = base.Joins("JOIN stores sf ON sf.id=p.store_id").Where("(p.occurred_at AT TIME ZONE sf.timezone)::date>=?::date", v)
	}
	if v := q["to"]; v != "" {
		base = base.Joins("JOIN stores st ON st.id=p.store_id").Where("(p.occurred_at AT TIME ZONE st.timezone)::date<=?::date", v)
	}
	var total int64
	if e := base.Count(&total).Error; e != nil {
		return nil, 0, e
	}
	var rows []queryRow
	selectDB := s.db.WithContext(ctx).Table("payments p").Where("p.store_id=? AND p.status='CONFIRMED'", store)
	if v := q["workerId"]; v != "" {
		selectDB = selectDB.Where("p.worker_id=?", v)
	}
	if v := q["search"]; v != "" {
		like := "%" + v + "%"
		selectDB = selectDB.Where("p.payment_no ILIKE ? OR w.name ILIKE ?", like, like)
	}
	if v := q["from"]; v != "" {
		selectDB = selectDB.Where("(p.occurred_at AT TIME ZONE s.timezone)::date>=?::date", v)
	}
	if v := q["to"]; v != "" {
		selectDB = selectDB.Where("(p.occurred_at AT TIME ZONE s.timezone)::date<=?::date", v)
	}
	if e := selectDB.Select(`p.*,w.name worker_name,u.name operator_name,COALESCE(le.balance_after,0) receivable_after`).Joins("JOIN workers w ON w.id=p.worker_id AND w.store_id=p.store_id").Joins("JOIN users u ON u.id=p.created_by AND u.store_id=p.store_id").Joins("JOIN stores s ON s.id=p.store_id").Joins(`LEFT JOIN ledger_entries le ON le.store_id=p.store_id AND ((le.type='PAYMENT' AND le.source_type='PAYMENT' AND le.source_id=p.id) OR (p.order_id IS NOT NULL AND le.type='ORDER_PAYMENT' AND le.source_type='ORDER' AND le.source_id=p.order_id))`).Order("p.occurred_at DESC,p.created_at DESC,p.id DESC").Offset(p.Offset()).Limit(p.PageSize).Scan(&rows).Error; e != nil {
		return nil, 0, e
	}
	ids := make([]uuid.UUID, len(rows))
	for i := range rows {
		ids[i] = rows[i].ID
	}
	allocations, e := s.allocations(ctx, store, ids)
	if e != nil {
		return nil, 0, e
	}
	items := make([]Response, len(rows))
	for i, row := range rows {
		allocated := decimal.Zero
		for _, a := range allocations[row.ID] {
			v, _ := decimal.NewFromString(a.Amount)
			allocated = allocated.Add(v)
		}
		items[i] = Response{ID: row.ID.String(), TransactionNo: row.PaymentNo, WorkerID: row.WorkerID.String(), WorkerName: row.WorkerName, Amount: row.Amount.StringFixed(2), PaymentMethod: row.PaymentMethod, OccurredAt: row.OccurredAt.Format(time.RFC3339), Note: row.Remark, OperatorName: row.OperatorName, CreatedAt: row.CreatedAt.Format(time.RFC3339), ReceivableBefore: row.ReceivableAfter.Add(row.Amount).StringFixed(2), ReceivableAfter: row.ReceivableAfter.StringFixed(2), UnallocatedAmount: row.Amount.Sub(allocated).StringFixed(2), Allocations: allocations[row.ID]}
		if items[i].Allocations == nil {
			items[i].Allocations = []AllocationResponse{}
		}
	}
	return items, total, nil
}

func (s *Service) allocations(ctx context.Context, store uuid.UUID, paymentIDs []uuid.UUID) (map[uuid.UUID][]AllocationResponse, error) {
	result := map[uuid.UUID][]AllocationResponse{}
	if len(paymentIDs) == 0 {
		return result, nil
	}
	var rows []struct {
		PaymentID, OrderID uuid.UUID
		OrderNo            string
		Amount             decimal.Decimal
	}
	if e := s.db.WithContext(ctx).Table("payment_allocations a").Select("a.payment_id,a.order_id,o.order_no,a.amount").Joins("JOIN orders o ON o.id=a.order_id AND o.store_id=a.store_id").Where("a.store_id=? AND a.payment_id IN ?", store, paymentIDs).Order("o.occurred_at,o.created_at,o.id").Scan(&rows).Error; e != nil {
		return nil, e
	}
	for _, row := range rows {
		result[row.PaymentID] = append(result[row.PaymentID], AllocationResponse{OrderID: row.OrderID.String(), OrderNo: row.OrderNo, Amount: row.Amount.StringFixed(2)})
	}
	return result, nil
}

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) List(c *gin.Context) {
	store, err := platformrequest.UUID(c, auth.ContextStoreID)
	if err != nil {
		response.WriteError(c, err)
		return
	}
	q := map[string]string{}
	for _, key := range []string{"search", "workerId", "from", "to"} {
		q[key] = c.Query(key)
	}
	p := pagination.Parse(c.Request.URL.Query())
	items, total, err := h.service.List(c.Request.Context(), store, q, p)
	if err != nil {
		response.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, pagination.Page[Response]{Items: items, Page: p.Page, PageSize: p.PageSize, Total: total})
}

func (h *Handler) Create(c *gin.Context) {
	key := c.GetHeader("Idempotency-Key")
	if key == "" || len(key) > 255 {
		response.WriteError(c, apperror.New(422, "IDEMPOTENCY_REQUIRED", "缺少防重复提交标识。"))
		return
	}
	store, err := platformrequest.UUID(c, auth.ContextStoreID)
	if err != nil {
		response.WriteError(c, err)
		return
	}
	user, err := platformrequest.UUID(c, auth.ContextUserID)
	if err != nil {
		response.WriteError(c, err)
		return
	}
	var input Input
	if c.ShouldBindJSON(&input) != nil {
		response.WriteError(c, apperror.Validation(map[string][]string{"body": {"请求格式不正确"}}))
		return
	}
	item, replayed, err := h.service.Create(c.Request.Context(), createMeta{StoreID: store, UserID: user, Key: key, RequestID: response.RequestID(c), IP: c.ClientIP(), UserAgent: c.Request.UserAgent()}, input)
	if err != nil {
		response.WriteError(c, err)
		return
	}
	if replayed {
		c.Header("Idempotent-Replayed", "true")
	}
	c.JSON(http.StatusCreated, item)
}

func RegisterRoutes(api *gin.RouterGroup, h *Handler, middleware auth.Middleware) {
	group := api.Group("/payments", middleware.Authenticate())
	group.GET("", auth.Require("finance:read"), h.List)
	group.POST("", auth.Require("payments:create"), h.Create)
}
