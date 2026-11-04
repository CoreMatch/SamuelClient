package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
)

// Logger returns a gin middleware for logging
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		gin.DefaultWriter.Write([]byte(
			"[GIN] " + start.Format("2006/01/02 - 15:04:05") +
				" | " + c.ClientIP() +
				" | " + c.Request.Method +
				" | " + path +
				" | " + latency.String() +
				" | " + string(rune(status)) + "\n",
		))
	}
}

// Recovery returns a gin middleware for panic recovery
func Recovery() gin.HandlerFunc {
	return gin.Recovery()
}
