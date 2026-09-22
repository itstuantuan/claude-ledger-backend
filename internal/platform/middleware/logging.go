package middleware

import (
	"time"

	platformlog "cloud-ledger-backend/internal/platform/logger"
	"cloud-ledger-backend/internal/platform/response"
	"github.com/gin-gonic/gin"
)

func AccessLog(log *platformlog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()
		log.Info("http request",
			platformlog.String("request_id", response.RequestID(c)),
			platformlog.String("user_id", contextString(c, "user_id")),
			platformlog.String("store_id", contextString(c, "store_id")),
			platformlog.String("method", c.Request.Method),
			platformlog.String("path", c.Request.URL.Path),
			platformlog.Int("status", c.Writer.Status()),
			platformlog.Duration("duration", time.Since(started)),
		)
	}
}

func contextString(c *gin.Context, key string) string {
	v, exists := c.Get(key)
	if !exists {
		return ""
	}
	s, _ := v.(string)
	return s
}
