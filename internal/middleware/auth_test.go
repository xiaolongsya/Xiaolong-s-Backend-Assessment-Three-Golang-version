package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAuthMiddleware_MissingHeader_Unauthorized(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(AuthMiddleware())
	r.GET("/ping", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestAuthMiddleware_InvalidToken_Unauthorized(t *testing.T) {
	gin.SetMode(gin.TestMode)

	old, had := os.LookupEnv("API_TOKENS")
	_ = os.Setenv("API_TOKENS", "a,b")
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("API_TOKENS", old)
		} else {
			_ = os.Unsetenv("API_TOKENS")
		}
	})

	r := gin.New()
	r.Use(AuthMiddleware())
	r.GET("/ping", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Authorization", "Bearer bad")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestAuthMiddleware_ValidToken_OK(t *testing.T) {
	gin.SetMode(gin.TestMode)

	old, had := os.LookupEnv("API_TOKENS")
	_ = os.Setenv("API_TOKENS", "a,b")
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("API_TOKENS", old)
		} else {
			_ = os.Unsetenv("API_TOKENS")
		}
	})

	r := gin.New()
	r.Use(AuthMiddleware())
	r.GET("/ping", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Authorization", "Bearer a")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, w.Code)
	}
}
