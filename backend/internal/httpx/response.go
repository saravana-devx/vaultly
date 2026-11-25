// Package httpx gives every handler the same JSON envelope:
// {"success": bool, "message": string, "data": any}. "data" is omitted when empty.
package httpx

import "github.com/gin-gonic/gin"

func Success(c *gin.Context, status int, message string, data any) {
	body := gin.H{"success": true, "message": message}
	if data != nil {
		body["data"] = data
	}
	c.JSON(status, body)
}

func Error(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"success": false, "message": message})
}

// AbortError is for middleware: it also stops the handler chain.
func AbortError(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, gin.H{"success": false, "message": message})
}
