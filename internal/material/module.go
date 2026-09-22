package material

import (
	"cloud-ledger-backend/internal/auth"
	"cloud-ledger-backend/internal/platform/apperror"
	"cloud-ledger-backend/internal/platform/pagination"
	platformrequest "cloud-ledger-backend/internal/platform/request"
	"cloud-ledger-backend/internal/platform/response"
	"context"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var money = regexp.MustCompile(`^\d+(?:\.\d{1,2})?$`)

type Model struct {
	ID                                         uuid.UUID
	StoreID                                    uuid.UUID
	Name, Category, Brand, Specification, Unit string
	DefaultSalePrice, CostPrice                decimal.Decimal
	Status                                     string
	SalesCount, Version                        int64
	CreatedAt, UpdatedAt                       time.Time
}

func (Model) TableName() string { return "materials" }

type Input struct {
	Name          string `json:"name"`
	Category      string `json:"category"`
	Brand         string `json:"brand"`
	Specification string `json:"specification"`
	Unit          string `json:"unit"`
	DefaultPrice  string `json:"defaultPrice"`
	CostPrice     string `json:"costPrice"`
	Status        string `json:"status"`
	Version       *int64 `json:"version,omitempty"`
}
type Response struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Category      string `json:"category"`
	Brand         string `json:"brand"`
	Specification string `json:"specification"`
	Unit          string `json:"unit"`
	DefaultPrice  string `json:"defaultPrice"`
	CostPrice     string `json:"costPrice"`
	Status        string `json:"status"`
	SalesCount    int64  `json:"salesCount"`
	Version       int64  `json:"version"`
}
type Service struct{ db *gorm.DB }

func NewService(db *gorm.DB) *Service { return &Service{db: db} }
func (s *Service) List(ctx context.Context, store uuid.UUID, q map[string]string, p pagination.Params) ([]Response, int64, error) {
	db := s.db.WithContext(ctx).Model(&Model{}).Where("store_id=?", store)
	if v := q["search"]; v != "" {
		like := "%" + v + "%"
		db = db.Where("name ILIKE ? OR brand ILIKE ? OR specification ILIKE ?", like, like, like)
	}
	for _, k := range []string{"brand", "category", "status"} {
		if v := q[k]; v != "" {
			db = db.Where(k+" = ?", v)
		}
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var models []Model
	if err := db.Order("created_at DESC").Offset(p.Offset()).Limit(p.PageSize).Find(&models).Error; err != nil {
		return nil, 0, err
	}
	items := make([]Response, len(models))
	for i, v := range models {
		items[i] = mapResponse(v)
	}
	return items, total, nil
}
func (s *Service) Create(ctx context.Context, store uuid.UUID, input Input) (Response, error) {
	def, cost, err := validate(input)
	if err != nil {
		return Response{}, err
	}
	now := time.Now().UTC()
	status := input.Status
	if status == "" {
		status = "ACTIVE"
	}
	m := Model{ID: uuid.New(), StoreID: store, Name: strings.TrimSpace(input.Name), Category: strings.TrimSpace(input.Category), Brand: strings.TrimSpace(input.Brand), Specification: strings.TrimSpace(input.Specification), Unit: strings.TrimSpace(input.Unit), DefaultSalePrice: def, CostPrice: cost, Status: status, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err = s.db.WithContext(ctx).Create(&m).Error; err != nil {
		return Response{}, err
	}
	return mapResponse(m), nil
}
func (s *Service) Update(ctx context.Context, store, id uuid.UUID, input Input) (Response, error) {
	def, cost, err := validate(input)
	if err != nil {
		return Response{}, err
	}
	if input.Version == nil {
		return Response{}, apperror.New(409, "VERSION_CONFLICT", "材料已被其他人修改，请刷新后重试。")
	}
	result := s.db.WithContext(ctx).Model(&Model{}).Where("store_id=? AND id=? AND version=?", store, id, *input.Version).Updates(map[string]any{"name": strings.TrimSpace(input.Name), "category": strings.TrimSpace(input.Category), "brand": strings.TrimSpace(input.Brand), "specification": strings.TrimSpace(input.Specification), "unit": strings.TrimSpace(input.Unit), "default_sale_price": def, "cost_price": cost, "status": input.Status, "version": gorm.Expr("version+1"), "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return Response{}, result.Error
	}
	if result.RowsAffected == 0 {
		var count int64
		s.db.WithContext(ctx).Model(&Model{}).Where("store_id=? AND id=?", store, id).Count(&count)
		if count == 0 {
			return Response{}, apperror.New(404, "MATERIAL_NOT_FOUND", "材料不存在。")
		}
		return Response{}, apperror.New(409, "VERSION_CONFLICT", "材料已被其他人修改，请刷新后重试。")
	}
	var m Model
	if err = s.db.WithContext(ctx).Where("store_id=? AND id=?", store, id).First(&m).Error; err != nil {
		return Response{}, err
	}
	return mapResponse(m), nil
}
func validate(i Input) (decimal.Decimal, decimal.Decimal, error) {
	bad := strings.TrimSpace(i.Name) == "" || len([]rune(i.Name)) > 80 || strings.TrimSpace(i.Category) == "" || len([]rune(i.Category)) > 40 || strings.TrimSpace(i.Brand) == "" || len([]rune(i.Brand)) > 40 || strings.TrimSpace(i.Specification) == "" || len([]rune(i.Specification)) > 40 || strings.TrimSpace(i.Unit) == "" || len([]rune(i.Unit)) > 12 || !money.MatchString(i.DefaultPrice) || !money.MatchString(i.CostPrice)
	if bad {
		return decimal.Zero, decimal.Zero, apperror.Validation(map[string][]string{"form": {"请检查材料资料。"}})
	}
	if i.Status != "" && i.Status != "ACTIVE" && i.Status != "DISABLED" {
		return decimal.Zero, decimal.Zero, apperror.Validation(map[string][]string{"status": {"状态无效"}})
	}
	d, _ := decimal.NewFromString(i.DefaultPrice)
	c, _ := decimal.NewFromString(i.CostPrice)
	return d, c, nil
}
func mapResponse(m Model) Response {
	return Response{ID: m.ID.String(), Name: m.Name, Category: m.Category, Brand: m.Brand, Specification: m.Specification, Unit: m.Unit, DefaultPrice: m.DefaultSalePrice.StringFixed(2), CostPrice: m.CostPrice.StringFixed(2), Status: m.Status, SalesCount: m.SalesCount, Version: m.Version}
}

type Handler struct{ service *Service }

func NewHandler(s *Service) *Handler { return &Handler{service: s} }
func (h *Handler) List(c *gin.Context) {
	store, err := platformrequest.UUID(c, auth.ContextStoreID)
	if err != nil {
		response.WriteError(c, err)
		return
	}
	q := map[string]string{}
	for _, k := range []string{"search", "brand", "category", "status"} {
		q[k] = c.Query(k)
	}
	p := pagination.Parse(c.Request.URL.Query())
	items, total, err := h.service.List(c.Request.Context(), store, q, p)
	if err != nil {
		response.WriteError(c, err)
		return
	}
	c.JSON(200, pagination.Page[Response]{Items: items, Page: p.Page, PageSize: p.PageSize, Total: total})
}
func (h *Handler) Create(c *gin.Context) {
	store, err := platformrequest.UUID(c, auth.ContextStoreID)
	if err != nil {
		response.WriteError(c, err)
		return
	}
	var input Input
	if c.ShouldBindJSON(&input) != nil {
		response.WriteError(c, apperror.Validation(map[string][]string{"body": {"请求格式不正确"}}))
		return
	}
	item, err := h.service.Create(c.Request.Context(), store, input)
	if err != nil {
		response.WriteError(c, err)
		return
	}
	c.JSON(http.StatusCreated, item)
}
func (h *Handler) Update(c *gin.Context) {
	store, err := platformrequest.UUID(c, auth.ContextStoreID)
	if err != nil {
		response.WriteError(c, err)
		return
	}
	id, err := platformrequest.ParamUUID(c, "id")
	if err != nil {
		response.WriteError(c, err)
		return
	}
	var input Input
	if c.ShouldBindJSON(&input) != nil {
		response.WriteError(c, apperror.Validation(map[string][]string{"body": {"请求格式不正确"}}))
		return
	}
	item, err := h.service.Update(c.Request.Context(), store, id, input)
	if err != nil {
		response.WriteError(c, err)
		return
	}
	c.JSON(200, item)
}
func RegisterRoutes(api *gin.RouterGroup, h *Handler, m auth.Middleware) {
	g := api.Group("/materials", m.Authenticate())
	g.GET("", auth.Require("materials:read"), h.List)
	g.POST("", auth.Require("materials:write"), h.Create)
	g.PATCH("/:id", auth.Require("materials:write"), h.Update)
}
