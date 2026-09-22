package pricing

import (
	"cloud-ledger-backend/internal/auth"
	"cloud-ledger-backend/internal/platform/apperror"
	platformrequest "cloud-ledger-backend/internal/platform/request"
	"cloud-ledger-backend/internal/platform/response"
	"context"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"net/http"
	"regexp"
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
}
type PriceInput struct {
	MaterialID string  `json:"materialId"`
	Price      *string `json:"price"`
}
type BatchInput struct {
	WorkerID string       `json:"workerId"`
	Prices   []PriceInput `json:"prices"`
}
type Service struct{ db *gorm.DB }

func NewService(db *gorm.DB) *Service { return &Service{db: db} }
func (s *Service) worker(ctx context.Context, store, worker uuid.UUID, lock bool) error {
	db := s.db.WithContext(ctx).Table("workers")
	if lock {
		db = db.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var id uuid.UUID
	if err := db.Select("id").Where("store_id=? AND id=?", store, worker).Scan(&id).Error; err != nil {
		return err
	}
	if id == uuid.Nil {
		return apperror.New(404, "WORKER_NOT_FOUND", "油漆工不存在。")
	}
	return nil
}
func (s *Service) List(ctx context.Context, store, worker uuid.UUID) ([]Item, error) {
	if err := s.worker(ctx, store, worker, false); err != nil {
		return nil, err
	}
	var rows []struct {
		MaterialID                               uuid.UUID
		MaterialName, Brand, Specification, Unit string
		DefaultPrice                             decimal.Decimal
		CustomerPrice                            *decimal.Decimal
	}
	err := s.db.WithContext(ctx).Table("materials m").Select("m.id AS material_id,m.name AS material_name,m.brand,m.specification,m.unit,m.default_sale_price AS default_price,cp.price AS customer_price").Joins("LEFT JOIN customer_prices cp ON cp.store_id=m.store_id AND cp.material_id=m.id AND cp.worker_id=? AND cp.status='ACTIVE' AND cp.effective_to IS NULL", worker).Where("m.store_id=? AND m.status='ACTIVE'", store).Order("m.created_at DESC").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	items := make([]Item, len(rows))
	for i, r := range rows {
		var customer *string
		effective := r.DefaultPrice
		if r.CustomerPrice != nil {
			v := r.CustomerPrice.StringFixed(2)
			customer = &v
			effective = *r.CustomerPrice
		}
		items[i] = Item{MaterialID: r.MaterialID.String(), MaterialName: r.MaterialName, Brand: r.Brand, Specification: r.Specification, Unit: r.Unit, DefaultPrice: r.DefaultPrice.StringFixed(2), CustomerPrice: customer, EffectivePrice: effective.StringFixed(2)}
	}
	return items, nil
}
func (s *Service) Save(ctx context.Context, store uuid.UUID, input BatchInput) ([]Item, error) {
	worker, err := uuid.Parse(input.WorkerID)
	if err != nil {
		return nil, apperror.Validation(map[string][]string{"workerId": {"请选择油漆工"}})
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		local := &Service{db: tx}
		if err := local.worker(ctx, store, worker, true); err != nil {
			return err
		}
		now := time.Now().UTC()
		seen := map[uuid.UUID]bool{}
		for _, p := range input.Prices {
			materialID, e := uuid.Parse(p.MaterialID)
			if e != nil || seen[materialID] {
				return apperror.Validation(map[string][]string{"prices": {"材料或价格无效"}})
			}
			seen[materialID] = true
			var count int64
			if e = tx.Table("materials").Where("store_id=? AND id=? AND status='ACTIVE'", store, materialID).Count(&count).Error; e != nil {
				return e
			}
			if count == 0 {
				return apperror.New(404, "MATERIAL_NOT_FOUND", "材料不存在。")
			}
			if e = tx.Model(&Model{}).Where("store_id=? AND worker_id=? AND material_id=? AND status='ACTIVE' AND effective_to IS NULL", store, worker, materialID).Updates(map[string]any{"effective_to": now, "status": "DISABLED", "updated_at": now}).Error; e != nil {
				return e
			}
			if p.Price != nil {
				if !money.MatchString(*p.Price) {
					return apperror.Validation(map[string][]string{"prices": {"价格格式不正确"}})
				}
				price, _ := decimal.NewFromString(*p.Price)
				m := Model{ID: uuid.New(), StoreID: store, WorkerID: worker, MaterialID: materialID, Price: price, EffectiveFrom: now, Status: "ACTIVE", Version: 1, CreatedAt: now, UpdatedAt: now}
				if e = tx.Create(&m).Error; e != nil {
					return e
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.List(ctx, store, worker)
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
	items, e := h.service.List(c.Request.Context(), store, worker)
	if e != nil {
		response.WriteError(c, e)
		return
	}
	c.JSON(200, items)
}
func (h *Handler) Save(c *gin.Context) {
	store, e := platformrequest.UUID(c, auth.ContextStoreID)
	if e != nil {
		response.WriteError(c, e)
		return
	}
	var input BatchInput
	if c.ShouldBindJSON(&input) != nil {
		response.WriteError(c, apperror.Validation(map[string][]string{"body": {"请求格式不正确"}}))
		return
	}
	items, e := h.service.Save(c.Request.Context(), store, input)
	if e != nil {
		response.WriteError(c, e)
		return
	}
	c.JSON(http.StatusOK, items)
}
func RegisterRoutes(api *gin.RouterGroup, h *Handler, m auth.Middleware) {
	g := api.Group("/pricing", m.Authenticate())
	g.GET("", auth.Require("materials:read"), h.List)
	g.PATCH("", auth.Require("materials:write"), h.Save)
}
