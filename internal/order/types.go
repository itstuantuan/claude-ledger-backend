package order

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type Input struct {
	WorkerID         string      `json:"workerId"`
	ProjectID        *string     `json:"projectId"`
	ProjectName      *string     `json:"projectName"`
	OccurredAt       string      `json:"occurredAt"`
	Items            []ItemInput `json:"items"`
	PaymentAmount    string      `json:"paymentAmount"`
	PrepaidDeduction string      `json:"prepaidDeduction"`
	PaymentMethod    *string     `json:"paymentMethod"`
	Note             string      `json:"note"`
}
type ItemInput struct {
	MaterialID string `json:"materialId"`
	Quantity   string `json:"quantity"`
	UnitPrice  string `json:"unitPrice"`
	Discount   string `json:"discount"`
}
type ItemResponse struct {
	MaterialID    string `json:"materialId"`
	MaterialName  string `json:"materialName"`
	Specification string `json:"specification"`
	Unit          string `json:"unit"`
	Quantity      string `json:"quantity"`
	UnitPrice     string `json:"unitPrice"`
	Discount      string `json:"discount"`
	Subtotal      string `json:"subtotal"`
}
type Response struct {
	ID                string         `json:"id"`
	OrderNo           string         `json:"orderNo"`
	WorkerID          string         `json:"workerId"`
	WorkerName        string         `json:"workerName"`
	ProjectID         *string        `json:"projectId"`
	ProjectName       *string        `json:"projectName"`
	Items             []ItemResponse `json:"items"`
	GoodsAmount       string         `json:"goodsAmount"`
	DiscountAmount    string         `json:"discountAmount"`
	FinalAmount       string         `json:"finalAmount"`
	PaymentAmount     string         `json:"paymentAmount"`
	PrepaidDeduction  string         `json:"prepaidDeduction"`
	AddedReceivable   string         `json:"addedReceivable"`
	ReturnedAmount    string         `json:"returnedAmount"`
	SettledAmount     string         `json:"settledAmount"`
	OutstandingAmount string         `json:"outstandingAmount"`
	PaymentMethod     *string        `json:"paymentMethod"`
	Status            string         `json:"status"`
	Note              string         `json:"note"`
	OccurredAt        string         `json:"occurredAt"`
	CreatedAt         string         `json:"createdAt"`
	OperatorName      string         `json:"operatorName"`
}

type Model struct {
	ID, StoreID                                                                                                                                        uuid.UUID
	OrderNo                                                                                                                                            string
	WorkerID                                                                                                                                           uuid.UUID
	WorkerNameSnapshot                                                                                                                                 string
	ProjectID                                                                                                                                          *uuid.UUID
	ProjectNameSnapshot                                                                                                                                *string
	Status                                                                                                                                             string
	GoodsAmount, DiscountAmount, FinalAmount, PaymentAmount, PrepaidDeductionAmount, AddedReceivable, ReturnedAmount, SettledAmount, OutstandingAmount decimal.Decimal
	PaymentMethod                                                                                                                                      *string
	Remark                                                                                                                                             string
	OccurredAt, ConfirmedAt                                                                                                                            time.Time
	CreatedBy                                                                                                                                          uuid.UUID
	CreatedAt, UpdatedAt                                                                                                                               time.Time
}

func (Model) TableName() string { return "orders" }

type ItemModel struct {
	ID, StoreID, OrderID, MaterialID                                         uuid.UUID
	MaterialNameSnapshot, BrandSnapshot, SpecificationSnapshot, UnitSnapshot string
	UnitPrice, Quantity, GrossAmount, DiscountAmount, Subtotal               decimal.Decimal
	PriceSource                                                              string
	CreatedAt                                                                time.Time
}

func (ItemModel) TableName() string { return "order_items" }

type WorkerAccount struct {
	ID, StoreID                                                    uuid.UUID
	Name, Status                                                   string
	MaterialTotal, PaymentTotal, PrepaidBalance, CurrentReceivable decimal.Decimal
	LastTransactionAt                                              *time.Time
	Version                                                        int64
}

func (WorkerAccount) TableName() string { return "workers" }

type PrepaidAccount struct {
	ID, StoreID, WorkerID uuid.UUID
	Balance               decimal.Decimal
	Version               int64
	CreatedAt, UpdatedAt  time.Time
}

func (PrepaidAccount) TableName() string { return "prepaid_accounts" }

type Payment struct {
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

func (Payment) TableName() string { return "payments" }

type LedgerEntry struct {
	ID, StoreID, WorkerID      uuid.UUID
	ProjectID                  *uuid.UUID
	LedgerNo, Type             string
	SignedAmount, BalanceAfter decimal.Decimal
	SourceType                 string
	SourceID                   uuid.UUID
	SourceNo                   string
	Remark                     string
	OccurredAt                 time.Time
	CreatedBy                  uuid.UUID
	CreatedAt                  time.Time
}

func (LedgerEntry) TableName() string { return "ledger_entries" }

type PrepaidTransaction struct {
	ID, StoreID, WorkerID                     uuid.UUID
	TransactionNo, Type                       string
	SignedAmount, BalanceBefore, BalanceAfter decimal.Decimal
	SourceType                                string
	SourceID                                  uuid.UUID
	Remark                                    string
	OccurredAt                                time.Time
	CreatedBy                                 uuid.UUID
	CreatedAt                                 time.Time
}

func (PrepaidTransaction) TableName() string { return "prepaid_transactions" }

type FinancialTransaction struct {
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

func (FinancialTransaction) TableName() string { return "financial_transactions" }

type IdempotencyRecord struct {
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

func (IdempotencyRecord) TableName() string { return "idempotency_records" }

type AuditLog struct {
	ID, StoreID, UserID      uuid.UUID
	Action, ResourceType     string
	ResourceID               uuid.UUID
	AfterData                []byte `gorm:"type:jsonb"`
	RequestID, IP, UserAgent string
	CreatedAt                time.Time
}

func (AuditLog) TableName() string { return "audit_logs" }
