package auth

import (
	"net/http"

	"cloud-ledger-backend/internal/platform/apperror"
	"cloud-ledger-backend/internal/platform/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const ContextUserID = "user_id"
const ContextStoreID = "store_id"
const ContextPermissions = "permissions"

type Middleware struct {
	repo   Repository
	tokens TokenManager
}

func NewMiddleware(repo Repository, tokens TokenManager) Middleware {
	return Middleware{repo: repo, tokens: tokens}
}

func (m Middleware) Authenticate() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, err := m.tokens.Parse(bearer(c.GetHeader("Authorization")))
		if err != nil {
			response.WriteError(c, apperror.New(http.StatusUnauthorized, "UNAUTHENTICATED", "请先登录。"))
			return
		}
		userID, err := uuid.Parse(claims.Subject)
		if err != nil {
			response.WriteError(c, apperror.New(401, "UNAUTHENTICATED", "请先登录。"))
			return
		}
		user, err := m.repo.FindUserByID(c.Request.Context(), userID)
		if err != nil || user.Status != "ACTIVE" || user.AuthVersion != claims.AuthVersion || user.StoreID.String() != claims.StoreID {
			response.WriteError(c, apperror.New(401, "UNAUTHENTICATED", "登录已过期，请重新登录。"))
			return
		}
		permissions := make(map[string]struct{}, len(user.Permissions))
		for _, permission := range user.Permissions {
			permissions[permission] = struct{}{}
		}
		c.Set(ContextUserID, user.ID.String())
		c.Set(ContextStoreID, user.StoreID.String())
		c.Set(ContextPermissions, permissions)
		c.Next()
	}
}

func Require(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		permissions, ok := c.Get(ContextPermissions)
		if !ok {
			response.WriteError(c, apperror.New(401, "UNAUTHENTICATED", "请先登录。"))
			return
		}
		if _, ok := permissions.(map[string]struct{})[permission]; !ok {
			response.WriteError(c, apperror.New(403, "FORBIDDEN", "你没有执行此操作的权限。"))
			return
		}
		c.Next()
	}
}
