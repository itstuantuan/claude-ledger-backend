package order

import (
	"cloud-ledger-backend/internal/auth"
	"cloud-ledger-backend/internal/platform/apperror"
	"cloud-ledger-backend/internal/platform/pagination"
	platformrequest "cloud-ledger-backend/internal/platform/request"
	"cloud-ledger-backend/internal/platform/response"
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

type Handler struct{ service *Service }

func NewHandler(s *Service) *Handler { return &Handler{service: s} }
func (h *Handler) List(c *gin.Context) {
	store, e := platformrequest.UUID(c, auth.ContextStoreID)
	if e != nil {
		response.WriteError(c, e)
		return
	}
	q := map[string]string{}
	for _, k := range []string{"search", "workerId", "status", "from", "to"} {
		q[k] = c.Query(k)
	}
	if !validDateRange(q["from"], q["to"]) {
		response.WriteError(c, apperror.New(422, "INVALID_DATE_RANGE", "查询日期范围不正确。"))
		return
	}
	p := pagination.Parse(c.Request.URL.Query())
	items, total, e := h.service.List(c.Request.Context(), store, q, p)
	if e != nil {
		response.WriteError(c, e)
		return
	}
	c.JSON(200, pagination.Page[Response]{Items: items, Page: p.Page, PageSize: p.PageSize, Total: total})
}
func validDateRange(from, to string) bool {
	var start, end time.Time
	var err error
	if from != "" {
		start, err = time.Parse("2006-01-02", from)
		if err != nil {
			return false
		}
	}
	if to != "" {
		end, err = time.Parse("2006-01-02", to)
		if err != nil {
			return false
		}
	}
	return from == "" || to == "" || !end.Before(start)
}
func (h *Handler) Get(c *gin.Context) {
	store, e := platformrequest.UUID(c, auth.ContextStoreID)
	if e != nil {
		response.WriteError(c, e)
		return
	}
	id, e := platformrequest.ParamUUID(c, "id")
	if e != nil {
		response.WriteError(c, e)
		return
	}
	item, e := h.service.Get(c.Request.Context(), store, id)
	if e != nil {
		response.WriteError(c, e)
		return
	}
	c.JSON(200, item)
}
func (h *Handler) Create(c *gin.Context) {
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
	var input Input
	if c.ShouldBindJSON(&input) != nil {
		response.WriteError(c, apperror.Validation(map[string][]string{"body": {"请求格式不正确"}}))
		return
	}
	item, replayed, e := h.service.Create(c.Request.Context(), createMeta{StoreID: store, UserID: user, Key: key, RequestID: response.RequestID(c), IP: c.ClientIP(), UserAgent: c.Request.UserAgent()}, input)
	if e != nil {
		response.WriteError(c, e)
		return
	}
	if replayed {
		c.Header("Idempotent-Replayed", "true")
	}
	c.JSON(http.StatusCreated, item)
}
func (h *Handler) ListAdjustments(c *gin.Context) {
	store, e := platformrequest.UUID(c, auth.ContextStoreID)
	if e != nil {
		response.WriteError(c, e)
		return
	}
	orderID, e := platformrequest.ParamUUID(c, "id")
	if e != nil {
		response.WriteError(c, e)
		return
	}
	items, e := h.service.ListAdjustments(c.Request.Context(), store, orderID)
	if e != nil {
		response.WriteError(c, e)
		return
	}
	c.JSON(http.StatusOK, items)
}
func (h *Handler) Adjust(c *gin.Context) {
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
	orderID, e := platformrequest.ParamUUID(c, "id")
	if e != nil {
		response.WriteError(c, e)
		return
	}
	var input AdjustmentInput
	if c.ShouldBindJSON(&input) != nil {
		response.WriteError(c, apperror.Validation(map[string][]string{"body": {"请求格式不正确"}}))
		return
	}
	item, replayed, e := h.service.Adjust(c.Request.Context(), createMeta{StoreID: store, UserID: user, Key: key, RequestID: response.RequestID(c), IP: c.ClientIP(), UserAgent: c.Request.UserAgent()}, orderID, input)
	if e != nil {
		response.WriteError(c, e)
		return
	}
	if replayed {
		c.Header("Idempotent-Replayed", "true")
	}
	c.JSON(http.StatusCreated, item)
}
func RegisterRoutes(api *gin.RouterGroup, h *Handler, m auth.Middleware) {
	g := api.Group("/orders", m.Authenticate())
	g.GET("", auth.Require("workers:read"), h.List)
	g.GET("/:id", auth.Require("workers:read"), h.Get)
	g.POST("", auth.Require("orders:create"), h.Create)
	g.GET("/:id/adjustments", auth.Require("workers:read"), h.ListAdjustments)
	g.POST("/:id/adjustments", auth.Require("orders:create"), h.Adjust)
}
