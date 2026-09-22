package worker

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
	ID                                                                          uuid.UUID
	StoreID                                                                     uuid.UUID
	TeamID                                                                      *uuid.UUID
	Name, Phone                                                                 string
	Wechat                                                                      *string
	Address                                                                     string
	Status, Remark                                                              string
	MaterialTotal, ReturnTotal, PaymentTotal, PrepaidBalance, CurrentReceivable decimal.Decimal
	LastTransactionAt                                                           *time.Time
	Version                                                                     int64
	CreatedBy                                                                   uuid.UUID
	CreatedAt, UpdatedAt                                                        time.Time
}

func (Model) TableName() string { return "workers" }

type Input struct {
	Name    string  `json:"name"`
	Phone   string  `json:"phone"`
	Wechat  string  `json:"wechat"`
	TeamID  *string `json:"teamId"`
	Status  string  `json:"status"`
	Note    string  `json:"note"`
	Version *int64  `json:"version,omitempty"`
}
type Response struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	Phone             string  `json:"phone"`
	Wechat            *string `json:"wechat"`
	TeamID            *string `json:"teamId"`
	TeamName          *string `json:"teamName"`
	Status            string  `json:"status"`
	MaterialTotal     string  `json:"materialTotal"`
	ReturnTotal       string  `json:"returnTotal"`
	PaymentTotal      string  `json:"paymentTotal"`
	PrepaidBalance    string  `json:"prepaidBalance"`
	Receivable        string  `json:"receivable"`
	LastTransactionAt *string `json:"lastTransactionAt"`
	Note              string  `json:"note"`
	Version           int64   `json:"version"`
}
type row struct {
	Model
	TeamName *string
}
type Service struct{ db *gorm.DB }

func NewService(db *gorm.DB) *Service { return &Service{db: db} }
func (s *Service) base(ctx context.Context, storeID uuid.UUID) *gorm.DB {
	return s.db.WithContext(ctx).Table("workers w").Select("w.*,t.name AS team_name").Joins("LEFT JOIN teams t ON t.id=w.team_id AND t.store_id=w.store_id").Where("w.store_id=?", storeID)
}
func (s *Service) List(ctx context.Context, storeID uuid.UUID, q map[string]string, p pagination.Params) ([]Response, int64, error) {
	base := s.db.WithContext(ctx).Table("workers w").Where("w.store_id=?", storeID)
	if v := q["search"]; v != "" {
		like := "%" + v + "%"
		base = base.Where("w.name ILIKE ? OR w.phone ILIKE ? OR w.wechat ILIKE ?", like, like, like)
	}
	if v := q["teamId"]; v != "" {
		base = base.Where("w.team_id=?", v)
	}
	if v := q["status"]; v != "" {
		base = base.Where("w.status=?", v)
	}
	if q["debt"] == "owing" {
		base = base.Where("w.current_receivable>0")
	} else if q["debt"] == "clear" {
		base = base.Where("w.current_receivable<=0")
	}
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	order := "w.last_transaction_at DESC NULLS LAST,w.created_at DESC"
	if q["sort"] == "receivable_desc" {
		order = "w.current_receivable DESC,w.created_at DESC"
	} else if q["sort"] == "name_asc" {
		order = "w.name ASC,w.created_at DESC"
	}
	var rows []row
	err := base.Select("w.*,t.name AS team_name").Joins("LEFT JOIN teams t ON t.id=w.team_id AND t.store_id=w.store_id").Order(order).Offset(p.Offset()).Limit(p.PageSize).Scan(&rows).Error
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
	err := s.base(ctx, storeID).Where("w.id=?", id).Scan(&item).Error
	if err != nil {
		return Response{}, err
	}
	if item.ID == uuid.Nil {
		return Response{}, apperror.New(404, "WORKER_NOT_FOUND", "油漆工不存在。")
	}
	return mapResponse(item), nil
}
func (s *Service) teamID(ctx context.Context, storeID uuid.UUID, raw *string) (*uuid.UUID, error) {
	if raw == nil || *raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(*raw)
	if err != nil {
		return nil, apperror.Validation(map[string][]string{"teamId": {"施工队无效"}})
	}
	var count int64
	if err = s.db.WithContext(ctx).Table("teams").Where("store_id=? AND id=?", storeID, id).Count(&count).Error; err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, apperror.New(404, "TEAM_NOT_FOUND", "施工队不存在。")
	}
	return &id, nil
}
func (s *Service) Create(ctx context.Context, storeID, userID uuid.UUID, input Input) (Response, error) {
	if err := validate(input); err != nil {
		return Response{}, err
	}
	teamID, err := s.teamID(ctx, storeID, input.TeamID)
	if err != nil {
		return Response{}, err
	}
	now := time.Now().UTC()
	status := input.Status
	if status == "" {
		status = "ACTIVE"
	}
	var wechat *string
	if value := strings.TrimSpace(input.Wechat); value != "" {
		wechat = &value
	}
	zero := decimal.Zero
	item := Model{ID: uuid.New(), StoreID: storeID, TeamID: teamID, Name: strings.TrimSpace(input.Name), Phone: strings.TrimSpace(input.Phone), Wechat: wechat, Status: status, Remark: strings.TrimSpace(input.Note), MaterialTotal: zero, ReturnTotal: zero, PaymentTotal: zero, PrepaidBalance: zero, CurrentReceivable: zero, Version: 1, CreatedBy: userID, CreatedAt: now, UpdatedAt: now}
	if err = s.db.WithContext(ctx).Create(&item).Error; err != nil {
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
	teamID, err := s.teamID(ctx, storeID, input.TeamID)
	if err != nil {
		return Response{}, err
	}
	var wechat any = nil
	if value := strings.TrimSpace(input.Wechat); value != "" {
		wechat = value
	}
	result := s.db.WithContext(ctx).Model(&Model{}).Where("store_id=? AND id=? AND version=?", storeID, id, *input.Version).Updates(map[string]any{"name": strings.TrimSpace(input.Name), "phone": strings.TrimSpace(input.Phone), "wechat": wechat, "team_id": teamID, "status": input.Status, "remark": strings.TrimSpace(input.Note), "version": gorm.Expr("version+1"), "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return Response{}, result.Error
	}
	if result.RowsAffected == 0 {
		var count int64
		s.db.WithContext(ctx).Model(&Model{}).Where("store_id=? AND id=?", storeID, id).Count(&count)
		if count == 0 {
			return Response{}, apperror.New(404, "WORKER_NOT_FOUND", "油漆工不存在.")
		}
		return Response{}, apperror.New(409, "VERSION_CONFLICT", "资料已被其他人修改，请刷新后重试。")
	}
	return s.Get(ctx, storeID, id)
}
func validate(i Input) error {
	if strings.TrimSpace(i.Name) == "" || len([]rune(i.Name)) > 30 || !validPhone(i.Phone) || len([]rune(i.Wechat)) > 50 || len([]rune(i.Note)) > 200 {
		return apperror.Validation(map[string][]string{"form": {"请检查油漆工资料。"}})
	}
	if i.Status != "" && i.Status != "ACTIVE" && i.Status != "DISABLED" {
		return apperror.Validation(map[string][]string{"status": {"状态无效"}})
	}
	return nil
}
func validPhone(v string) bool {
	v = strings.TrimSpace(v)
	if len(v) != 11 || v[0] != '1' {
		return false
	}
	for _, c := range v {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
func mapResponse(i row) Response {
	var teamID *string
	if i.TeamID != nil {
		v := i.TeamID.String()
		teamID = &v
	}
	var last *string
	if i.LastTransactionAt != nil {
		v := i.LastTransactionAt.Format(time.RFC3339)
		last = &v
	}
	return Response{ID: i.ID.String(), Name: i.Name, Phone: i.Phone, Wechat: i.Wechat, TeamID: teamID, TeamName: i.TeamName, Status: i.Status, MaterialTotal: i.MaterialTotal.StringFixed(2), ReturnTotal: i.ReturnTotal.StringFixed(2), PaymentTotal: i.PaymentTotal.StringFixed(2), PrepaidBalance: i.PrepaidBalance.StringFixed(2), Receivable: i.CurrentReceivable.StringFixed(2), LastTransactionAt: last, Note: i.Remark, Version: i.Version}
}

type Handler struct{ service *Service }

func NewHandler(s *Service) *Handler { return &Handler{service: s} }
func (h *Handler) List(c *gin.Context) {
	storeID, err := platformrequest.UUID(c, auth.ContextStoreID)
	if err != nil {
		response.WriteError(c, err)
		return
	}
	q := map[string]string{}
	for _, k := range []string{"search", "teamId", "debt", "status", "sort"} {
		q[k] = c.Query(k)
	}
	p := pagination.Parse(c.Request.URL.Query())
	items, total, err := h.service.List(c.Request.Context(), storeID, q, p)
	if err != nil {
		response.WriteError(c, err)
		return
	}
	c.JSON(200, pagination.Page[Response]{Items: items, Page: p.Page, PageSize: p.PageSize, Total: total})
}
func (h *Handler) Get(c *gin.Context) {
	store, id, err := ids(c)
	if err != nil {
		response.WriteError(c, err)
		return
	}
	item, err := h.service.Get(c.Request.Context(), store, id)
	if err != nil {
		response.WriteError(c, err)
		return
	}
	c.JSON(200, item)
}
func (h *Handler) Create(c *gin.Context) {
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
	item, err := h.service.Create(c.Request.Context(), store, user, input)
	if err != nil {
		response.WriteError(c, err)
		return
	}
	c.JSON(http.StatusCreated, item)
}
func (h *Handler) Update(c *gin.Context) {
	store, id, err := ids(c)
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
func ids(c *gin.Context) (uuid.UUID, uuid.UUID, error) {
	s, e := platformrequest.UUID(c, auth.ContextStoreID)
	if e != nil {
		return uuid.Nil, uuid.Nil, e
	}
	id, e := platformrequest.ParamUUID(c, "id")
	return s, id, e
}
func RegisterRoutes(api *gin.RouterGroup, h *Handler, m auth.Middleware) {
	g := api.Group("/workers", m.Authenticate())
	g.GET("", auth.Require("workers:read"), h.List)
	g.GET("/:id", auth.Require("workers:read"), h.Get)
	g.POST("", auth.Require("workers:write"), h.Create)
	g.PATCH("/:id", auth.Require("workers:write"), h.Update)
}
