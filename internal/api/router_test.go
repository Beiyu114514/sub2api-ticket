package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAccessLogOmitsQueryString(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	old := gin.DefaultWriter
	gin.DefaultWriter = &buf
	defer func() { gin.DefaultWriter = old }()

	r := gin.New()
	r.Use(accessLog())
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/?user_id=1&token=secret-jwt&theme=light", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	out := buf.String()
	if strings.Contains(out, "secret-jwt") {
		t.Fatalf("token leaked into access log: %s", out)
	}
	if strings.Contains(out, "token=") || strings.Contains(out, "user_id=") {
		t.Fatalf("query string should not be logged: %s", out)
	}
	if !strings.Contains(out, "\"/\"") {
		t.Fatalf("request path should still be logged: %s", out)
	}
}

func TestAccessLogSkipsWebhookSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	old := gin.DefaultWriter
	gin.DefaultWriter = &buf
	defer func() { gin.DefaultWriter = old }()

	r := gin.New()
	r.Use(accessLog())
	r.POST("/api/v1/telegram/webhook/:secret", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/api/v1/telegram/webhook/super-secret", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if buf.Len() != 0 {
		t.Fatalf("webhook path must not be logged, got: %s", buf.String())
	}
}
