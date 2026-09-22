package health

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	db      pinger
	timeout time.Duration
}

type pinger interface{ PingContext(context.Context) error }

func New(db *sql.DB, timeout time.Duration) Handler { return Handler{db: db, timeout: timeout} }

func NewWithPinger(db pinger, timeout time.Duration) Handler {
	return Handler{db: db, timeout: timeout}
}

func (h Handler) Get(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.timeout)
	defer cancel()
	databaseStatus, status := "up", http.StatusOK
	if err := h.db.PingContext(ctx); err != nil {
		databaseStatus, status = "down", http.StatusServiceUnavailable
	}
	c.JSON(status, gin.H{"application": "up", "database": databaseStatus})
}
