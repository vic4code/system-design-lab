package middleware

import (
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// SecurityHeaders adds standard defensive HTTP headers to every response.
//
// In production these map to:
//   - WAF (AWS WAF on ALB): blocks common OWASP attacks
//   - CloudFront: strips / passes through these headers
//   - HSTS preload: tells browsers to always use HTTPS (365 days)
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Prevent MIME-type sniffing attacks
		c.Header("X-Content-Type-Options", "nosniff")

		// Prevent clickjacking — disallow embedding in <iframe>
		c.Header("X-Frame-Options", "DENY")

		// Enable browser XSS filter (legacy browsers)
		c.Header("X-XSS-Protection", "1; mode=block")

		// Strict Transport Security — enforces HTTPS for 1 year
		// Only effective when served over HTTPS (nginx/ALB terminates TLS)
		c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

		// Content Security Policy — restrict where resources can be loaded from
		// Adjust when adding analytics, CDN, etc.
		c.Header("Content-Security-Policy", buildCSP())

		// Don't send Referer header on cross-origin requests
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")

		// Restrict browser feature access
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")

		c.Next()
	}
}

func buildCSP() string {
	// In production the frontend origin comes from ALLOWED_ORIGINS env var
	frontendOrigin := getAllowedOrigin()

	directives := []string{
		"default-src 'self'",
		"script-src 'self'",
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data: blob:",
		"media-src 'self' blob:",
		"connect-src 'self' " + frontendOrigin,
		"font-src 'self'",
		"object-src 'none'",
		"base-uri 'self'",
		"frame-ancestors 'none'",
	}
	return strings.Join(directives, "; ")
}

// CORS returns a CORS middleware that restricts allowed origins.
//
// ALLOWED_ORIGINS env var: comma-separated list of allowed origins.
// Default (dev): permissive (all localhost ports).
// Production: set to the exact frontend domain, e.g. https://beatstream.example.com
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if isAllowedOrigin(origin) {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
		c.Header("Access-Control-Max-Age", "86400")
		c.Header("Access-Control-Allow-Credentials", "false")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

func getAllowedOrigin() string {
	if ao := os.Getenv("ALLOWED_ORIGINS"); ao != "" {
		// Return first origin for CSP (or * if explicitly set)
		parts := strings.Split(ao, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	return "http://localhost:3000 http://localhost:3001"
}

func isAllowedOrigin(origin string) bool {
	if origin == "" {
		return false
	}
	ao := os.Getenv("ALLOWED_ORIGINS")
	if ao == "" {
		// Dev: allow any localhost
		return strings.HasPrefix(origin, "http://localhost") ||
			strings.HasPrefix(origin, "https://localhost")
	}
	for _, allowed := range strings.Split(ao, ",") {
		if strings.TrimSpace(allowed) == origin {
			return true
		}
	}
	return false
}
