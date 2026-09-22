package team

import (
	"context"
	"net/http"
	"strings"
	"time"

	"cloud-ledger-backend/internal/auth"
	"cloud-ledger-backend/internal/platform/apperror"
	"cloud-ledger-backend/internal/platform/pagination"
	platformrequest "cloud-ledger-backend/internal/platform/request"
	"cloud-ledger-backend/internal/platform/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type Model struct {
	ID         uuid.UUID
	StoreID    uuid.UUID
	Name       string
	LeaderName string
	Phone      string
	Status     string
	Remark     string
	Version    int64
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (Model) TableName() string { return "teams" }

type Input struct {
	Name    string `json:"name"`
	Leader  string `json:"leader"`
	Phone   string `json:"phone"`
	Status  string `json:"status"`
	Note    string `json:"note"`
	Version *int64 `json:"version,omitempty"`
}
type Response struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Leader        string `json:"leader"`
	Phone         string `json:"phone"`
	MemberCount   int64  `json:"memberCount"`
	MaterialTotal string `json:"materialTotal"`
	PaymentTotal  string `json:"paymentTotal"`
	Receivable    string `json:"receivable"`
	Note          string `json:"note"`
	Status        string `json:"status"`
	Version       int64  `json:"version"`
}
type row struct {
	Model
	MemberCount   int64
	MaterialTotal decimal.Decimal
	PaymentTotal  decimal.Decimal
	Receivable    decimal.Decimal
}

type Service struct{ db *gorm.DB }

func NewService(db *gorm.DB) *Service { return &Service{db: db} }

func (s *Service) List(ctx context.Context, storeID uuid.UUID, search string, page pagination.Params) ([]Response, int64, error) {
	base := s.db.WithContext(ctx).Table("teams t").Where("t.store_id = ?", storeID)
	if search != "" {
		base = base.Where("t.name ILIKE ? OR t.leader_name ILIKE ? OR t.phone ILIKE ?", "%"+search+"%", "%"+search+"%", "%"+search+"%")
	}
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []row
	err := base.Select(`t.*, COUNT(w.id) AS member_count, COALESCE(SUM(w.material_total),0) AS material_total, COALESCE(SUM(w.payment_total),0) AS payment_total, COALESCE(SUM(w.current_receivable),0) AS receivable`).Joins("LEFT JOIN workers w ON w.team_id=t.id AND w.store_id=t.store_id").Group("t.id").Order("t.created_at DESC").Offset(page.Offset()).Limit(page.PageSize).Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	result := make([]Response, len(rows))
	for i, item := range rows {
		result[i] = mapResponse(item)
	}
	return result, total, nil
}

func (s *Service) Get(ctx context.Context, storeID, id uuid.UUID) (Response, error) {
	var item row
	err := s.db.WithContext(ctx).Table("teams t").Select(`t.*, COUNT(w.id) AS member_count, COALESCE(SUM(w.material_total),0) AS material_total, COALESCE(SUM(w.payment_total),0) AS payment_total, COALESCE(SUM(w.current_receivable),0) AS receivable`).Joins("LEFT JOIN workers w ON w.team_id=t.id AND w.store_id=t.store_id").Where("t.store_id=? AND t.id=?", storeID, id).Group("t.id").Scan(&item).Error
	if err != nil {
		return Response{}, err
	}
	if item.ID == uuid.Nil {
		return Response{}, apperror.New(404, "TEAM_NOT_FOUND", "施工队不存在。")
	}
	return mapResponse(item), nil
}

func (s *Service) Create(ctx context.Context, storeID uuid.UUID, input Input) (Response, error) {
	if err := validate(input); err != nil {
		return Response{}, err
	}
	now := time.Now().UTC()
	status := input.Status
	if status == "" {
		status = "ACTIVE"
	}
	item := Model{ID: uuid.New(), StoreID: storeID, Name: strings.TrimSpace(input.Name), LeaderName: strings.TrimSpace(input.Leader), Phone: strings.TrimSpace(input.Phone), Status: status, Remark: strings.TrimSpace(input.Note), Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := s.db.WithContext(ctx).Create(&item).Error; err != nil {
		return Response{}, err
	}
	return s.Get(ctx, storeID, item.ID)
}

func (s *Service) Update(ctx context.Context, storeID, id uuid.UUID, input Input) (Response, error) {
	if err := validate(input); err != nil {
		return Response{}, err
	}
	if input.Version == nil {
		return Response{}, apperror.New(409, "VERSION_CONFLICT", "资料已被其他人修改，请刷新后重试。")
	}
	result := s.db.WithContext(ctx).Model(&Model{}).Where("store_id=? AND id=? AND version=?", storeID, id, *input.Version).Updates(map[string]any{"name": strings.TrimSpace(input.Name), "leader_name": strings.TrimSpace(input.Leader), "phone": strings.TrimSpace(input.Phone), "status": input.Status, "remark": strings.TrimSpace(input.Note), "version": gorm.Expr("version+1"), "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return Response{}, result.Error
	}
	if result.RowsAffected == 0 {
		var count int64
		s.db.WithContext(ctx).Model(&Model{}).Where("store_id=? AND id=?", storeID, id).Count(&count)
		if count == 0 {
			return Response{}, apperror.New(404, "TEAM_NOT_FOUND", "施工队不存在。")
		}
		return Response{}, apperror.New(409, "VERSION_CONFLICT", "资料已被其他人修改，请刷新后重试。")
	}
	return s.Get(ctx, storeID, id)
}

func validate(input Input) error {
	if strings.TrimSpace(input.Name) == "" || len([]rune(input.Name)) > 50 || strings.TrimSpace(input.Leader) == "" || len([]rune(input.Leader)) > 30 || !validPhone(input.Phone) || len([]rune(input.Note)) > 200 {
		return apperror.Validation(map[string][]string{"form": {"请检查施工队资料。"}})
	}
	if input.Status != "" && input.Status != "ACTIVE" && input.Status != "DISABLED" {
		return apperror.Validation(map[string][]string{"status": {"状态无效"}})
	}
	return nil
}
func validPhone(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 11 || value[0] != '1' {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}
func mapResponse(item row) Response {
	return Response{ID: item.ID.String(), Name: item.Name, Leader: item.LeaderName, Phone: item.Phone, MemberCount: item.MemberCount, MaterialTotal: item.MaterialTotal.StringFixed(2), PaymentTotal: item.PaymentTotal.StringFixed(2), Receivable: item.Receivable.StringFixed(2), Note: item.Remark, Status: item.Status, Version: item.Version}
}

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }
func (h *Handler) List(c *gin.Context) {
	storeID, err := platformrequest.UUID(c, auth.ContextStoreID)
	if err != nil {
		response.WriteError(c, err)
		return
	}
	p := pagination.Parse(c.Request.URL.Query())
	items, total, err := h.service.List(c.Request.Context(), storeID, c.Query("search"), p)
	if err != nil {
		response.WriteError(c, err)
		return
	}
	c.JSON(200, pagination.Page[Response]{Items: items, Page: p.Page, PageSize: p.PageSize, Total: total})
}
func (h *Handler) Get(c *gin.Context) {
	storeID, id, err := ids(c)
	if err != nil {
		response.WriteError(c, err)
		return
	}
	item, err := h.service.Get(c.Request.Context(), storeID, id)
	if err != nil {
		response.WriteError(c, err)
		return
	}
	c.JSON(200, item)
}
func (h *Handler) Create(c *gin.Context) {
	storeID, err := platformrequest.UUID(c, auth.ContextStoreID)
	if err != nil {
		response.WriteError(c, err)
		return
	}
	var input Input
	if c.ShouldBindJSON(&input) != nil {
		response.WriteError(c, apperror.Validation(map[string][]string{"body": {"请求格式不正确"}}))
		return
	}
	item, err := h.service.Create(c.Request.Context(), storeID, input)
	if err != nil {
		response.WriteError(c, err)
		return
	}
	c.JSON(http.StatusCreated, item)
}
func (h *Handler) Update(c *gin.Context) {
	storeID, id, err := ids(c)
	if err != nil {
		response.WriteError(c, err)
		return
	}
	var input Input
	if c.ShouldBindJSON(&input) != nil {
		response.WriteError(c, apperror.Validation(map[string][]string{"body": {"请求格式不正确"}}))
		return
	}
	item, err := h.service.Update(c.Request.Context(), storeID, id, input)
	if err != nil {
		response.WriteError(c, err)
		return
	}
	c.JSON(200, item)
}
func ids(c *gin.Context) (uuid.UUID, uuid.UUID, error) {
	storeID, err := platformrequest.UUID(c, auth.ContextStoreID)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	id, err := platformrequest.ParamUUID(c, "id")
	return storeID, id, err
}
func RegisterRoutes(api *gin.RouterGroup, h *Handler, m auth.Middleware) {
	g := api.Group("/teams", m.Authenticate())
	g.GET("", auth.Require("workers:read"), h.List)
	g.GET("/:id", auth.Require("workers:read"), h.Get)
	g.POST("", auth.Require("workers:write"), h.Create)
	g.PATCH("/:id", auth.Require("workers:write"), h.Update)
}
