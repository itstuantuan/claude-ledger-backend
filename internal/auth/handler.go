package auth

import (
	"net/http"
	"strings"

	"cloud-ledger-backend/internal/platform/apperror"
	"cloud-ledger-backend/internal/platform/config"
	"cloud-ledger-backend/internal/platform/response"
	platformvalidation "cloud-ledger-backend/internal/platform/validation"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

type Handler struct {
	service  *Service
	validate *validator.Validate
	cfg      config.Auth
}

func NewHandler(service *Service, validate *validator.Validate, cfg config.Auth) *Handler {
	return &Handler{service: service, validate: validate, cfg: cfg}
}

func (h *Handler) Login(c *gin.Context) {
	var request LoginRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.WriteError(c, apperror.Validation(map[string][]string{"body": {"请求格式不正确"}}))
		return
	}
	if err := h.validate.Struct(request); err != nil {
		response.WriteError(c, apperror.Validation(platformvalidation.FieldErrors(err)))
		return
	}
	session, refresh, err := h.service.Login(c.Request.Context(), request, c.Request.UserAgent(), c.ClientIP())
	if err != nil {
		response.WriteError(c, err)
		return
	}
	h.setRefreshCookie(c, refresh)
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, session)
}

func (h *Handler) Refresh(c *gin.Context) {
	current, _ := c.Cookie(h.cfg.CookieName)
	session, refresh, err := h.service.Refresh(c.Request.Context(), current, c.Request.UserAgent(), c.ClientIP())
	if err != nil {
		h.clearRefreshCookie(c)
		response.WriteError(c, err)
		return
	}
	h.setRefreshCookie(c, refresh)
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, session)
}

func (h *Handler) Logout(c *gin.Context) {
	current, _ := c.Cookie(h.cfg.CookieName)
	if err := h.service.Logout(c.Request.Context(), current); err != nil {
		response.WriteError(c, err)
		return
	}
	h.clearRefreshCookie(c)
	c.Status(http.StatusNoContent)
}

func (h *Handler) Me(c *gin.Context) {
	raw, exists := c.Get(ContextUserID)
	if !exists {
		response.WriteError(c, apperror.New(401, "UNAUTHENTICATED", "请先登录。"))
		return
	}
	id, err := uuid.Parse(raw.(string))
	if err != nil {
		response.WriteError(c, apperror.New(401, "UNAUTHENTICATED", "请先登录。"))
		return
	}
	user, err := h.service.Me(c.Request.Context(), id)
	if err != nil {
		response.WriteError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, user)
}

func (h *Handler) setRefreshCookie(c *gin.Context, value string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(h.cfg.CookieName, value, int(h.cfg.RefreshExpires.Seconds()), "/api/v1/auth", "", h.cfg.CookieSecure, true)
}

func (h *Handler) clearRefreshCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(h.cfg.CookieName, "", -1, "/api/v1/auth", "", h.cfg.CookieSecure, true)
}

func bearer(header string) string {
	parts := strings.SplitN(header, " ", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return strings.TrimSpace(parts[1])
	}
	return ""
}
