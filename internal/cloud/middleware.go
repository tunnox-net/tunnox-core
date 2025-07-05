package cloud

import (
	"net/http"
	"tunnox-core/internal/constants"

	"github.com/gin-gonic/gin"
)

// AuthMiddleware 认证中间件
func AuthMiddleware(cloudControl CloudControlAPI) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader(constants.HTTPHeaderAuthorization)
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			c.Abort()
			return
		}

		if len(authHeader) < 7 || authHeader[:7] != "Bearer " {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authorization format"})
			c.Abort()
			return
		}

		token := authHeader[7:]
		resp, err := cloudControl.ValidateToken(c.Request.Context(), token)
		if err != nil || resp == nil || !resp.Success {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}

		// 注入用户/客户端信息到 gin.Context
		if resp.Client != nil {
			if resp.Client.UserID != "" {
				c.Set("user_id", resp.Client.UserID)
			}
			if resp.Client.ID != "" {
				c.Set("client_id", resp.Client.ID)
			}
		}
		c.Next()
	}
}
