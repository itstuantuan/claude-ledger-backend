package middleware

import (
	"cloud-ledger-backend/internal/platform/response"
	"github.com/gin-gonic/gin"
)

func Errors() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if c.Writer.Written() || len(c.Errors) == 0 {
			return
		}
		response.WriteError(c, c.Errors.Last().Err)
	}
}
