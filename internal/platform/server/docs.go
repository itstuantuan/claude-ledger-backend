package server

import (
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed openapi.yaml
var openAPI []byte

func registerDocs(router *gin.Engine) {
	router.GET("/openapi.yaml", func(c *gin.Context) { c.Data(http.StatusOK, "application/yaml; charset=utf-8", openAPI) })
	router.GET("/docs", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(`<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><title>云记账 API</title></head><body><h1>云记账 API</h1><p>OpenAPI 合同：<a href="/openapi.yaml">/openapi.yaml</a></p><p>Phase 2 当前提供 Health 与 Auth；业务端点将在对应阶段加入。</p></body></html>`))
	})
}
