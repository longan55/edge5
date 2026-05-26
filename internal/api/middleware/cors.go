package middleware

import (
	"edge5/config"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func CORS() gin.HandlerFunc {
	cfg := cors.DefaultConfig()
	cfg.AllowAllOrigins = true
	cfg.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	cfg.AllowHeaders = []string{"Origin", "Content-Type", "Accept", "Authorization"}

	if config.CONFIG.Server.Mode == "release" {
		cfg.AllowOrigins = []string{"http://localhost:3000"}
	}

	return cors.New(cfg)
}
