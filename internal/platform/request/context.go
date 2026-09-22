package request

import (
	"cloud-ledger-backend/internal/platform/apperror"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func UUID(c *gin.Context, key string) (uuid.UUID, error) {
	raw, ok := c.Get(key)
	if !ok {
		return uuid.Nil, apperror.New(401, "UNAUTHENTICATED", "请先登录。")
	}
	value, err := uuid.Parse(raw.(string))
	if err != nil {
		return uuid.Nil, apperror.New(401, "UNAUTHENTICATED", "请先登录。")
	}
	return value, nil
}

func ParamUUID(c *gin.Context, name string) (uuid.UUID, error) {
	value, err := uuid.Parse(c.Param(name))
	if err != nil {
		return uuid.Nil, apperror.New(404, "NOT_FOUND", "所请求的内容不存在。")
	}
	return value, nil
}
