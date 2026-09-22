package response

import (
	"errors"
	"net/http"

	"cloud-ledger-backend/internal/platform/apperror"
	"github.com/gin-gonic/gin"
)

type ErrorBody struct {
	Code        string              `json:"code"`
	Message     string              `json:"message"`
	RequestID   string              `json:"requestId"`
	FieldErrors map[string][]string `json:"fieldErrors,omitempty"`
}

func WriteError(c *gin.Context, err error) {
	appErr := apperror.New(http.StatusInternalServerError, "INTERNAL_ERROR", "服务暂时不可用，请稍后重试。")
	var target *apperror.Error
	if errors.As(err, &target) {
		appErr = target
	}
	c.AbortWithStatusJSON(appErr.HTTPStatus, ErrorBody{Code: appErr.Code, Message: appErr.Message, RequestID: RequestID(c), FieldErrors: appErr.FieldErrors})
}

func RequestID(c *gin.Context) string {
	if id, ok := c.Get("request_id"); ok {
		if value, ok := id.(string); ok {
			return value
		}
	}
	return ""
}
