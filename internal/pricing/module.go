package pricing

import (
	"bytes"
	"cloud-ledger-backend/internal/auth"
	"cloud-ledger-backend/internal/platform/apperror"
	platformrequest "cloud-ledger-backend/internal/platform/request"
	"cloud-ledger-backend/internal/platform/response"
	platformvalue "cloud-ledger-backend/internal/platform/value"
	"context"
	"crypto/sha256"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var money = regexp.MustCompile(`^\d+(?:\.\d{1,2})?$`)

type Model struct {
	ID, StoreID, WorkerID, MaterialID uuid.UUID
	Price                             decimal.Decimal
	EffectiveFrom                     time.Time
	EffectiveTo                       *time.Time
	Status                            string
	Version                           int64
	CreatedAt, UpdatedAt              time.Time
}

func (Model) TableName() string { return "customer_prices" }

type Item struct {
	MaterialID     string  `json:"materialId"`
	MaterialName   string  `json:"materialName"`
	Brand          string  `json:"brand"`
	Specification  string  `json:"specification"`
	Unit           string  `json:"unit"`
	DefaultPrice   string  `json:"defaultPrice"`
	CustomerPrice  *string `json:"customerPrice"`
	EffectivePrice string  `json:"effectivePrice"`
	PriceVersion   *int64  `json:"priceVersion"`
	EffectiveFrom  *string `json:"effectiveFrom"`
}
type PriceInput struct {
	MaterialID      string  `json:"materialId"`
	Price           *string `json:"price"`
	ExpectedVersion *int64  `json:"expectedVersion"`
}
type BatchInput struct {
	WorkerID string       `json:"workerId"`
	Prices   []PriceInput `json:"prices"`
}
type saveMeta struct {
	StoreID, UserID               uuid.UUID
	Key, RequestID, IP, UserAgent string
}
type idempotencyRecord struct {
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

func (idempotencyRecord) TableName() string { return "idempotency_records" }

type auditLog struct {
	ID, StoreID, UserID      uuid.UUID
	Action, ResourceType     string
	ResourceID               uuid.UUID
	BeforeData, AfterData    []byte `gorm:"type:jsonb"`
	RequestID, IP, UserAgent string
	CreatedAt                time.Time
}

func (auditLog) TableName() string { return "audit_logs" }

type Service struct {
	db  *gorm.DB
	now func() time.Time
}

func NewService(db *gorm.DB) *Service { return &Service{db: db, now: time.Now} }
func (s *Service) worker(ctx context.Context, store, worker uuid.UUID, lock bool) error {
	db := s.db.WithContext(ctx).Table("workers")
	if lock {
		db = db.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var row struct{ ID uuid.UUID }
	if err := db.Select("id").Where("store_id=? AND id=? AND status='ACTIVE'", store, worker).Scan(&row).Error; err != nil {
		return err
	}
	if row.ID == uuid.Nil {
		return apperror.New(404, "WORKER_NOT_FOUND", "油漆工不存在或已停用。")
	}
	return nil
}

func (s *Service) listAt(ctx context.Context, store, worker uuid.UUID, at time.Time, search string) ([]Item, error) {
	if err := s.worker(ctx, store, worker, false); err != nil {
		return nil, err
	}
	type materialRow struct {
		ID                               uuid.UUID
		Name, Brand, Specification, Unit string
		DefaultSalePrice                 decimal.Decimal
	}
	var materials []materialRow
	query := s.db.WithContext(ctx).Table("materials").Select("id,name,brand,specification,unit,default_sale_price").Where("store_id=? AND status='ACTIVE'", store)
	if search = strings.TrimSpace(search); search != "" {
		like := "%" + search + "%"
		query = query.Where("name ILIKE ? OR brand ILIKE ? OR specification ILIKE ?", like, like, like)
	}
	if err := query.Order("created_at DESC").Find(&materials).Error; err != nil {
		return nil, err
	}
	if len(materials) == 0 {
		return []Item{}, nil
	}
	ids := make([]uuid.UUID, len(materials))
	for i := range materials {
		ids[i] = materials[i].ID
	}
	var prices []Model
	if err := s.db.WithContext(ctx).Where("store_id=? AND worker_id=? AND material_id IN ? AND effective_from<=? AND (effective_to IS NULL OR effective_to>?)", store, worker, ids, at, at).Order("effective_from DESC").Find(&prices).Error; err != nil {
		return nil, err
	}
	byMaterial := map[uuid.UUID]Model{}
	for _, price := range prices {
		if _, exists := byMaterial[price.MaterialID]; !exists {
			byMaterial[price.MaterialID] = price
		}
	}
	var latest []Model
	if err := s.db.WithContext(ctx).Where("store_id=? AND worker_id=? AND material_id IN ?", store, worker, ids).Order("effective_from DESC").Find(&latest).Error; err != nil {
		return nil, err
	}
	latestVersion := map[uuid.UUID]int64{}
	for _, price := range latest {
		if _, exists := latestVersion[price.MaterialID]; !exists {
			latestVersion[price.MaterialID] = price.Version
		}
	}
	items := make([]Item, len(materials))
	for i, material := range materials {
		item := Item{MaterialID: material.ID.String(), MaterialName: material.Name, Brand: material.Brand, Specification: material.Specification, Unit: material.Unit, DefaultPrice: material.DefaultSalePrice.StringFixed(2), EffectivePrice: material.DefaultSalePrice.StringFixed(2)}
		if price, ok := byMaterial[material.ID]; ok {
			value, from, version := price.Price.StringFixed(2), price.EffectiveFrom.Format(time.RFC3339), price.Version
			item.CustomerPrice, item.EffectivePrice, item.PriceVersion, item.EffectiveFrom = &value, value, &version, &from
		} else if version, ok := latestVersion[material.ID]; ok {
			item.PriceVersion = &version
		}
		items[i] = item
	}
	return items, nil
}

func (s *Service) List(ctx context.Context, store, worker uuid.UUID, rawAt, search string) ([]Item, error) {
	at := s.now().UTC()
	if strings.TrimSpace(rawAt) != "" {
		var zone string
		if err := s.db.WithContext(ctx).Table("stores").Select("timezone").Where("id=?", store).Scan(&zone).Error; err != nil {
			return nil, err
		}
		parsed, _, err := platformvalue.ParseBusinessTime(rawAt, zone)
		if err != nil {
			return nil, apperror.Validation(map[string][]string{"at": {"价格查询时间格式不正确"}})
		}
		at = parsed
	}
	return s.listAt(ctx, store, worker, at, search)
}

func (s *Service) Save(ctx context.Context, meta saveMeta, input BatchInput) ([]Item, bool, error) {
	worker, err := uuid.Parse(input.WorkerID)
	if err != nil {
		return nil, false, apperror.Validation(map[string][]string{"workerId": {"请选择油漆工"}})
	}
	if len(input.Prices) == 0 {
		return nil, false, apperror.Validation(map[string][]string{"prices": {"没有需要保存的价格变更"}})
	}
	payload, _ := json.Marshal(input)
	hash := sha256.Sum256(payload)
	var output []Item
	replayed := false
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := s.now().UTC()
		record := idempotencyRecord{ID: uuid.New(), StoreID: meta.StoreID, UserID: meta.UserID, IdempotencyKey: meta.Key, Operation: "UPDATE_CUSTOMER_PRICING", RequestHash: hash[:], Status: "PROCESSING", CreatedAt: now, ExpiresAt: now.Add(24 * time.Hour)}
		insert := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&record)
		if insert.Error != nil {
			return insert.Error
		}
		if insert.RowsAffected == 0 {
			var old idempotencyRecord
			if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("store_id=? AND idempotency_key=?", meta.StoreID, meta.Key).First(&old).Error; e != nil {
				return e
			}
			if old.Operation != record.Operation || !bytes.Equal(old.RequestHash, hash[:]) {
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
		local := &Service{db: tx, now: s.now}
		if e := local.worker(ctx, meta.StoreID, worker, true); e != nil {
			return e
		}
		seen := map[uuid.UUID]bool{}
		before, after := map[string]any{}, map[string]any{}
		for _, requested := range input.Prices {
			materialID, e := uuid.Parse(requested.MaterialID)
			if e != nil || seen[materialID] {
				return apperror.Validation(map[string][]string{"prices": {"材料或价格无效"}})
			}
			seen[materialID] = true
			var count int64
			if e = tx.Table("materials").Where("store_id=? AND id=? AND status='ACTIVE'", meta.StoreID, materialID).Count(&count).Error; e != nil {
				return e
			}
			if count == 0 {
				return apperror.New(404, "MATERIAL_NOT_FOUND", "材料不存在。")
			}
			var current Model
			find := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("store_id=? AND worker_id=? AND material_id=? AND status='ACTIVE' AND effective_to IS NULL", meta.StoreID, worker, materialID).First(&current)
			hasCurrent := find.Error == nil
			if find.Error != nil && find.Error != gorm.ErrRecordNotFound {
				return find.Error
			}
			latestVersion := int64(0)
			if hasCurrent {
				latestVersion = current.Version
			} else {
				var latest Model
				lookup := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("store_id=? AND worker_id=? AND material_id=?", meta.StoreID, worker, materialID).Order("effective_from DESC").First(&latest)
				if lookup.Error == nil {
					latestVersion = latest.Version
				} else if lookup.Error != gorm.ErrRecordNotFound {
					return lookup.Error
				}
			}
			if latestVersion > 0 && (requested.ExpectedVersion == nil || *requested.ExpectedVersion != latestVersion) {
				return apperror.New(409, "PRICE_VERSION_CONFLICT", "客户价格已被其他人修改，请刷新后重试。")
			}
			if latestVersion == 0 && requested.ExpectedVersion != nil {
				return apperror.New(409, "PRICE_VERSION_CONFLICT", "客户价格已被其他人修改，请刷新后重试。")
			}
			var next *decimal.Decimal
			if requested.Price != nil {
				trimmed := strings.TrimSpace(*requested.Price)
				if !money.MatchString(trimmed) {
					return apperror.Validation(map[string][]string{"prices": {"价格格式不正确"}})
				}
				value, _ := decimal.NewFromString(trimmed)
				next = &value
			}
			if hasCurrent && next != nil && current.Price.Equal(*next) {
				continue
			}
			if !hasCurrent && next == nil {
				continue
			}
			var oldValue any
			if hasCurrent {
				oldValue = current.Price.StringFixed(2)
			}
			before[materialID.String()] = oldValue
			var newValue any
			if next != nil {
				newValue = next.StringFixed(2)
			}
			after[materialID.String()] = newValue
			nextVersion := latestVersion + 1
			if hasCurrent {
				updates := map[string]any{"effective_to": now, "status": "DISABLED", "updated_at": now}
				if next == nil {
					updates["version"] = nextVersion
				}
				if e = tx.Model(&Model{}).Where("id=? AND version=?", current.ID, current.Version).Updates(updates).Error; e != nil {
					return e
				}
			}
			if next != nil {
				model := Model{ID: uuid.New(), StoreID: meta.StoreID, WorkerID: worker, MaterialID: materialID, Price: *next, EffectiveFrom: now, Status: "ACTIVE", Version: nextVersion, CreatedAt: now, UpdatedAt: now}
				if e = tx.Create(&model).Error; e != nil {
					return e
				}
			}
		}
		items, e := local.listAt(ctx, meta.StoreID, worker, now, "")
		if e != nil {
			return e
		}
		output = items
		beforeJSON, _ := json.Marshal(before)
		afterJSON, _ := json.Marshal(after)
		audit := auditLog{ID: uuid.New(), StoreID: meta.StoreID, UserID: meta.UserID, Action: "pricing.batch_update", ResourceType: "WORKER", ResourceID: worker, BeforeData: beforeJSON, AfterData: afterJSON, RequestID: meta.RequestID, IP: meta.IP, UserAgent: meta.UserAgent, CreatedAt: now}
		if e = tx.Create(&audit).Error; e != nil {
			return e
		}
		body, _ := json.Marshal(output)
		resource := "WORKER"
		status := http.StatusOK
		return tx.Model(&idempotencyRecord{}).Where("id=?", record.ID).Updates(map[string]any{"resource_type": resource, "resource_id": worker, "response_status": status, "response_data": body, "status": "COMPLETED"}).Error
	})
	return output, replayed, err
}

type Handler struct{ service *Service }

func NewHandler(s *Service) *Handler { return &Handler{service: s} }
func (h *Handler) List(c *gin.Context) {
	store, e := platformrequest.UUID(c, auth.ContextStoreID)
	if e != nil {
		response.WriteError(c, e)
		return
	}
	worker, e := uuid.Parse(c.Query("workerId"))
	if e != nil {
		response.WriteError(c, apperror.New(422, "WORKER_REQUIRED", "请选择油漆工。"))
		return
	}
	items, e := h.service.List(c.Request.Context(), store, worker, c.Query("at"), c.Query("search"))
	if e != nil {
		response.WriteError(c, e)
		return
	}
	c.JSON(http.StatusOK, items)
}
func (h *Handler) Save(c *gin.Context) {
	key := c.GetHeader("Idempotency-Key")
	if key == "" || len(key) > 255 {
		response.WriteError(c, apperror.New(422, "IDEMPOTENCY_REQUIRED", "缺少防重复提交标识。"))
		return
	}
	store, e := platformrequest.UUID(c, auth.ContextStoreID)
	if e != nil {
		response.WriteError(c, e)
		return
	}
	user, e := platformrequest.UUID(c, auth.ContextUserID)
	if e != nil {
		response.WriteError(c, e)
		return
	}
	var input BatchInput
	if c.ShouldBindJSON(&input) != nil {
		response.WriteError(c, apperror.Validation(map[string][]string{"body": {"请求格式不正确"}}))
		return
	}
	items, replayed, e := h.service.Save(c.Request.Context(), saveMeta{StoreID: store, UserID: user, Key: key, RequestID: response.RequestID(c), IP: c.ClientIP(), UserAgent: c.Request.UserAgent()}, input)
	if e != nil {
		response.WriteError(c, e)
		return
	}
	if replayed {
		c.Header("Idempotent-Replayed", "true")
	}
	c.JSON(http.StatusOK, items)
}
func RegisterRoutes(api *gin.RouterGroup, h *Handler, m auth.Middleware) {
	g := api.Group("/pricing", m.Authenticate())
	g.GET("", auth.Require("materials:read"), h.List)
	g.PATCH("", auth.Require("materials:write"), h.Save)
}
