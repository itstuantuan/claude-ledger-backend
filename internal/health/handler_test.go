package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type fakePinger struct{ err error }

func (f fakePinger) PingContext(context.Context) error { return f.err }

func TestHealthReportsDatabaseState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, item := range map[string]struct {
		err    error
		status int
		body   string
	}{
		"up":   {nil, http.StatusOK, `{"application":"up","database":"up"}`},
		"down": {errors.New("offline"), http.StatusServiceUnavailable, `{"application":"up","database":"down"}`},
	} {
		t.Run(name, func(t *testing.T) {
			r := gin.New()
			handler := NewWithPinger(fakePinger{item.err}, time.Second)
			r.GET("/health", handler.Get)
			res := httptest.NewRecorder()
			r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/health", nil))
			if res.Code != item.status || res.Body.String() != item.body {
				t.Fatalf("got %d %s", res.Code, res.Body.String())
			}
		})
	}
}
