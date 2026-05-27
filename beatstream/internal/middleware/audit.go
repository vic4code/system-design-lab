// Package middleware provides the audit logging middleware.
//
// Every state-changing request (POST / PUT / PATCH / DELETE) is recorded in
// the audit_logs table after the handler completes. The record captures:
//   - who made the request (user_id from JWT, or "" for anonymous)
//   - what they did (HTTP method + path → normalised action string)
//   - what resource was affected (resource_type, resource_id parsed from path)
//   - request metadata (IP, User-Agent, status code, request ID)
//
// AWS mapping: this table is the application-level audit trail.
// At the cloud layer CloudTrail covers AWS API calls and ALB access logs
// cover all inbound HTTP traffic (both stored in S3 for 90 days).
package middleware

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	applogger "github.com/vic4code/system-design-lab/beatstream/internal/logger"
)

// AuditLog returns a gin middleware that writes an audit_logs row for every
// mutating request (POST, PUT, PATCH, DELETE) after the handler finishes.
// Read-only requests (GET, HEAD, OPTIONS) are skipped.
func AuditLog(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		// Only audit writes
		if method == "GET" || method == "HEAD" || method == "OPTIONS" {
			c.Next()
			return
		}

		// Run the handler first so we can capture the response status
		c.Next()

		// Capture context values set by JWT middleware
		userID, _ := c.Get("user_id")
		userIDStr, _ := userID.(string)

		action := buildAction(method, c.FullPath())
		resourceType, resourceID := parseResource(c.FullPath(), c)
		requestID := c.GetString("request_id")

		// Insert asynchronously so we don't add latency to the response.
		// We use context.Background() because the request context may be cancelled.
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			_, err := pool.Exec(ctx, `
				INSERT INTO audit_logs
				  (user_id, action, resource_type, resource_id,
				   ip_address, user_agent, status_code, request_id)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			`,
				nullableStr(userIDStr),
				action,
				resourceType,
				nullableStr(resourceID),
				c.ClientIP(),
				c.Request.UserAgent(),
				c.Writer.Status(),
				nullableStr(requestID),
			)
			if err != nil {
				applogger.L().Warn("audit log insert failed",
					zap.String("action", action),
					zap.Error(err),
				)
			}
		}()
	}
}

// buildAction converts method + route template into a readable action string.
// e.g. POST /v1/playlists/:id/tracks → "playlist.track.add"
func buildAction(method, routeTemplate string) string {
	// Remove /v1/ prefix and clean up
	path := strings.TrimPrefix(routeTemplate, "/v1/")
	parts := strings.Split(path, "/")

	switch {
	case method == "POST" && path == "auth/register":
		return "auth.register"
	case method == "POST" && path == "auth/login":
		return "auth.login"
	case method == "DELETE" && strings.HasPrefix(path, "users/me"):
		return "user.delete"
	case method == "GET" && path == "users/me/data":
		return "user.data_export"
	case method == "POST" && len(parts) == 1:
		return parts[0][:len(parts[0])-1] + ".create" // "tracks" → "track.create"
	case method == "DELETE" && len(parts) == 2:
		return parts[0][:len(parts[0])-1] + ".delete"
	case method == "POST" && len(parts) == 3 && parts[2] == "tracks":
		return parts[0][:len(parts[0])-1] + ".track.add"
	case method == "DELETE" && len(parts) == 4 && parts[2] == "tracks":
		return parts[0][:len(parts[0])-1] + ".track.remove"
	default:
		return strings.ToLower(method) + "." + strings.ReplaceAll(path, "/", ".")
	}
}

// parseResource extracts the primary resource type and ID from the route.
func parseResource(routeTemplate string, c *gin.Context) (resourceType, resourceID string) {
	path := strings.TrimPrefix(routeTemplate, "/v1/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return "unknown", ""
	}

	switch parts[0] {
	case "tracks":
		resourceType = "track"
	case "playlists":
		resourceType = "playlist"
	case "artists":
		resourceType = "artist"
	case "auth":
		resourceType = "user"
	case "users":
		resourceType = "user"
	default:
		resourceType = parts[0]
	}

	// Try to get the primary resource ID from the URL
	if len(parts) >= 2 {
		resourceID = c.Param("id")
	}
	return
}

func nullableStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
