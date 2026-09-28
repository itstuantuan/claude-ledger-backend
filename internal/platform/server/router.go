package server

import (
	"cloud-ledger-backend/internal/auth"
	"cloud-ledger-backend/internal/health"
	"cloud-ledger-backend/internal/material"
	"cloud-ledger-backend/internal/order"
	"cloud-ledger-backend/internal/payment"
	"cloud-ledger-backend/internal/platform/config"
	platformlog "cloud-ledger-backend/internal/platform/logger"
	platformmw "cloud-ledger-backend/internal/platform/middleware"
	platformvalidation "cloud-ledger-backend/internal/platform/validation"
	"cloud-ledger-backend/internal/pricing"
	"cloud-ledger-backend/internal/project"
	"cloud-ledger-backend/internal/team"
	"cloud-ledger-backend/internal/worker"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"gorm.io/gorm"
)

func NewRouter(cfg config.Config, log *platformlog.Logger, db *gorm.DB) *gin.Engine {
	if cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.TestMode)
	}
	router := gin.New()
	_ = router.SetTrustedProxies(nil)
	binding.EnableDecoderDisallowUnknownFields = true
	router.Use(platformmw.RequestID(), platformmw.Recovery(log), platformmw.AccessLog(log), platformmw.Errors(), platformmw.SecurityHeaders(), platformmw.CORS(cfg.CORSOrigins))
	sqlDB, _ := db.DB()
	healthHandler := health.New(sqlDB, cfg.Database.PingTimeout)
	router.GET("/health", healthHandler.Get)
	registerDocs(router)
	repository := auth.NewRepository(db)
	tokens := auth.NewTokenManager(cfg.Auth.AccessSecret, cfg.Auth.AccessExpires)
	authService := auth.NewService(repository, tokens, cfg.Auth.RefreshExpires)
	authHandler := auth.NewHandler(authService, platformvalidation.New(), cfg.Auth)
	api := router.Group("/api/v1")
	authMiddleware := auth.NewMiddleware(repository, tokens)
	auth.RegisterRoutes(api, authHandler, authMiddleware)
	team.RegisterRoutes(api, team.NewHandler(team.NewService(db)), authMiddleware)
	worker.RegisterRoutes(api, worker.NewHandler(worker.NewService(db)), authMiddleware)
	project.RegisterRoutes(api, project.NewHandler(project.NewService(db)), authMiddleware)
	material.RegisterRoutes(api, material.NewHandler(material.NewService(db)), authMiddleware)
	pricing.RegisterRoutes(api, pricing.NewHandler(pricing.NewService(db)), authMiddleware)
	order.RegisterRoutes(api, order.NewHandler(order.NewService(db)), authMiddleware)
	payment.RegisterRoutes(api, payment.NewHandler(payment.NewService(db)), authMiddleware)
	return router
}
