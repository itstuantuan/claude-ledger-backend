package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"cloud-ledger-backend/internal/platform/apperror"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func TestRequestIDIsReturnedAndUnsafeValueIsReplaced(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID())
	r.GET("/", func(c *gin.Context) { c.Status(204) })
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "unsafe value")
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	if got := res.Header().Get("X-Request-ID"); got == "" || got == "unsafe value" {
		t.Fatalf("unexpected request id %q", got)
	}
}

func TestCORSAllowsConfiguredOriginAndRejectsOthers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORS([]string{"http://localhost:3000"}))
	r.GET("/", func(c *gin.Context) { c.Status(204) })
	for origin, status := range map[string]int{"http://localhost:3000": 204, "http://evil.test": 403} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Origin", origin)
		res := httptest.NewRecorder()
		r.ServeHTTP(res, req)
		if res.Code != status {
			t.Fatalf("origin %s: got %d want %d", origin, res.Code, status)
		}
	}
}

func TestAppErrorAndPanicUseStableErrorBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID(), Recovery(zap.NewNop()), Errors())
	r.GET("/business", func(c *gin.Context) { _ = c.Error(apperror.New(409, "VERSION_CONFLICT", "资料已变更。")) })
	r.GET("/panic", func(c *gin.Context) { panic("secret internal detail") })
	for path, code := range map[string]string{"/business": `"code":"VERSION_CONFLICT"`, "/panic": `"code":"INTERNAL_ERROR"`} {
		res := httptest.NewRecorder()
		r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, path, nil))
		if res.Code < 400 || !contains(res.Body.String(), code) || !contains(res.Body.String(), `"requestId":`) {
			t.Fatalf("%s: %d %s", path, res.Code, res.Body.String())
		}
		if contains(res.Body.String(), "secret internal detail") {
			t.Fatal("panic detail leaked")
		}
	}
}

func contains(value, fragment string) bool {
	for i := 0; i+len(fragment) <= len(value); i++ {
		if value[i:i+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
