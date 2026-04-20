package middleware

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

func allowedTokens() map[string]struct{} {
	raw := strings.TrimSpace(os.Getenv("API_TOKENS"))
	if raw == "" {
		return map[string]struct{}{"test-token": {}}
	}

	tokens := make(map[string]struct{})
	for _, part := range strings.Split(raw, ",") {
		t := strings.TrimSpace(part)
		if t == "" {
			continue
		}
		tokens[t] = struct{}{}
	}
	if len(tokens) == 0 {
		return map[string]struct{}{"test-token": {}}
	}
	return tokens
}

// AuthMiddleware 校验 Bearer Token
func AuthMiddleware() gin.HandlerFunc {
	allowed := allowedTokens()
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"message": "Missing or invalid Authorization header",
					"type":    "authentication_error",
				},
			})
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		token = strings.TrimSpace(token)

		if _, ok := allowed[token]; !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"message": "Invalid API key",
					"type":    "authentication_error",
				},
			})
			return
		}

		c.Next()
	}
}
