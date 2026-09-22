package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cloud-ledger-backend/internal/platform/config"
	"cloud-ledger-backend/internal/platform/middleware"
	platformvalidation "cloud-ledger-backend/internal/platform/validation"
	"github.com/gin-gonic/gin"
)

func testAuthRouter(t *testing.T, repo *fakeRepository) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := config.Auth{AccessSecret: "test-access-secret-at-least-32-bytes", AccessExpires: 15 * time.Minute, RefreshExpires: 7 * 24 * time.Hour, CookieName: "cloud_ledger_refresh"}
	tokens := NewTokenManager(cfg.AccessSecret, cfg.AccessExpires)
	service := NewService(repo, tokens, cfg.RefreshExpires)
	handler := NewHandler(service, platformvalidation.New(), cfg)
	router := gin.New()
	router.Use(middleware.RequestID(), middleware.Errors())
	RegisterRoutes(router.Group("/api/v1"), handler, NewMiddleware(repo, tokens))
	return router
}

func TestLoginHTTPContractAndCookie(t *testing.T) {
	router := testAuthRouter(t, &fakeRepository{user: testUser(t)})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"account":"owner","password":"Paint123!"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body.String())
	}
	var session SessionResponse
	if err := json.Unmarshal(response.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if session.User.Account != "owner" || session.AccessToken == "" {
		t.Fatalf("unexpected body: %+v", session)
	}
	cookie := response.Header().Get("Set-Cookie")
	for _, expected := range []string{"cloud_ledger_refresh=", "HttpOnly", "SameSite=Lax", "Path=/api/v1/auth"} {
		if !strings.Contains(cookie, expected) {
			t.Fatalf("cookie missing %s: %s", expected, cookie)
		}
	}
}

func TestLoginValidationMatchesFrontendErrorShape(t *testing.T) {
	router := testAuthRouter(t, &fakeRepository{user: testUser(t)})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"account":"","password":""}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 422 || !strings.Contains(response.Body.String(), `"code":"VALIDATION_ERROR"`) || !strings.Contains(response.Body.String(), `"fieldErrors"`) {
		t.Fatalf("unexpected response: %d %s", response.Code, response.Body.String())
	}
}

func TestAuthenticateRejectsAuthVersionChangeAndRequireChecksPermission(t *testing.T) {
	repo := &fakeRepository{user: testUser(t)}
	manager := NewTokenManager("test-access-secret-at-least-32-bytes", 15*time.Minute)
	token, _, err := manager.Access(repo.user, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	authn := NewMiddleware(repo, manager)
	router.GET("/allowed", authn.Authenticate(), Require("system:manage"), func(c *gin.Context) { c.Status(204) })
	router.GET("/denied", authn.Authenticate(), Require("payments:create"), func(c *gin.Context) { c.Status(204) })
	call := func(path string) int {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		return res.Code
	}
	if status := call("/allowed"); status != 204 {
		t.Fatalf("allowed status %d", status)
	}
	if status := call("/denied"); status != 403 {
		t.Fatalf("denied status %d", status)
	}
	repo.user.AuthVersion++
	if status := call("/allowed"); status != 401 {
		t.Fatalf("stale token status %d", status)
	}
}
