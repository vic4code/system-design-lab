package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const UserIDKey = "user_id"
const UserEmailKey = "user_email"
const UserNameKey = "user_name"

// RequireAuth rejects requests without a valid JWT.
func RequireAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractToken(c)
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		claims, err := parseToken(token, secret)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}

		c.Set(UserIDKey, claims["user_id"])
		c.Set(UserEmailKey, claims["email"])
		c.Set(UserNameKey, claims["name"])
		c.Next()
	}
}

// OptionalAuth parses the JWT if present but does not reject missing tokens.
// Use on endpoints that work for both guests and authenticated users.
func OptionalAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if token := extractToken(c); token != "" {
			if claims, err := parseToken(token, secret); err == nil {
				c.Set(UserIDKey, claims["user_id"])
				c.Set(UserEmailKey, claims["email"])
				c.Set(UserNameKey, claims["name"])
			}
		}
		c.Next()
	}
}

func extractToken(c *gin.Context) string {
	// Authorization: Bearer <token>
	if h := c.GetHeader("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	// Fallback: cookie
	if cookie, err := c.Cookie("token"); err == nil {
		return cookie
	}
	return ""
}

func parseToken(tokenStr, secret string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil || !token.Valid {
		return nil, err
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, jwt.ErrTokenInvalidClaims
	}
	return claims, nil
}
