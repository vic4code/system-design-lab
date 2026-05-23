package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const HeaderRequestID = "X-Request-ID"

// RequestID attaches a unique request ID to every request.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(HeaderRequestID)
		if id == "" {
			id = uuid.New().String()
		}
		c.Set("request_id", id)
		c.Header(HeaderRequestID, id)
		c.Next()
	}
}
