package middleware

import (
	"edge5/internal/utils/jwt"
	"edge5/internal/utils/response"
	"strings"

	"github.com/gin-gonic/gin"
)

func JWTAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth == "" {
			response.ErrorWithCode(c, 401, response.CodeUnauthorized, "未提供认证令牌")
			c.Abort()
			return
		}

		parts := strings.SplitN(auth, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			response.ErrorWithCode(c, 401, response.CodeUnauthorized, "认证令牌格式错误")
			c.Abort()
			return
		}

		tokenString := parts[1]
		claims, err := jwt.ParseToken(tokenString)
		if err != nil {
			if err == jwt.ErrTokenExpired {
				response.ErrorWithCode(c, 401, response.CodeTokenExpired, "令牌已过期")
			} else {
				response.ErrorWithCode(c, 401, response.CodeTokenInvalid, "令牌无效")
			}
			c.Abort()
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role_id", claims.RoleID)
		c.Set("role_code", claims.RoleCode)

		c.Next()
	}
}

func GetUserID(c *gin.Context) uint64 {
	userID, exists := c.Get("user_id")
	if !exists {
		return 0
	}
	return userID.(uint64)
}

func GetUsername(c *gin.Context) string {
	username, exists := c.Get("username")
	if !exists {
		return ""
	}
	return username.(string)
}

func GetRoleCode(c *gin.Context) string {
	roleCode, exists := c.Get("role_code")
	if !exists {
		return ""
	}
	return roleCode.(string)
}
