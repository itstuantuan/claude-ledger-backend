package middleware

import (
	"fmt"

	"cloud-ledger-backend/internal/platform/apperror"
	platformlog "cloud-ledger-backend/internal/platform/logger"
	"cloud-ledger-backend/internal/platform/response"
	"github.com/gin-gonic/gin"
)

func Recovery(log *platformlog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Error("panic recovered", platformlog.String("request_id", response.RequestID(c)), platformlog.String("panic", fmt.Sprint(recovered)))
				response.WriteError(c, apperror.New(500, "INTERNAL_ERROR", "服务暂时不可用，请稍后重试。"))
			}
		}()
		c.Next()
	}
}
