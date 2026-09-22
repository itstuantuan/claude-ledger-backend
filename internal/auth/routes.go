package auth

import "github.com/gin-gonic/gin"

func RegisterRoutes(api *gin.RouterGroup, handler *Handler, middleware Middleware) {
	group := api.Group("/auth")
	group.POST("/login", handler.Login)
	group.POST("/refresh", handler.Refresh)
	group.POST("/logout", handler.Logout)
	group.GET("/me", middleware.Authenticate(), handler.Me)
}
