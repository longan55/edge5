package router

import (
	"edge5/global"
	"edge5/internal/api/middleware"
	"edge5/internal/handler"
	"edge5/internal/repository"
	"edge5/internal/service"

	"github.com/gin-gonic/gin"
)

func SetupRouter(mode string) *gin.Engine {
	if mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.Logger())
	r.Use(middleware.CORS())

	userRepo := repository.NewUserRepository(global.DB, global.Logger)
	userService := service.NewUserService(userRepo, global.Logger)

	authHandler := handler.NewAuthHandler(userService)
	userHandler := handler.NewUserHandler(userService)

	// mqttRepo := repository.NewMQTTConfigRepository(global.DB)
	// mqttService := service.NewMQTTService(mqttRepo, global.Logger)
	// mqttHandler := handler.NewMQTTHandler(mqttService)

	deviceRepo := repository.NewDeviceRepository(global.DB)
	deviceStatusRepo := repository.NewDeviceStatusRepository(global.DB)
	deviceService := service.NewDeviceService(deviceRepo, deviceStatusRepo)
	deviceHandler := handler.NewDeviceHandler(deviceService)

	r.GET("/api/captcha", authHandler.GetCaptcha)

	api := r.Group("/api")
	{
		api.POST("/login", authHandler.Login)
		api.POST("/register", authHandler.Register)

		protected := api.Group("")
		protected.Use(middleware.JWTAuth())
		{
			protected.GET("/user/info", authHandler.GetUserInfo)

			user := protected.Group("/user")
			{
				user.GET("/list", userHandler.List)
				user.POST("", userHandler.Create)
				user.PUT("/:id", userHandler.Update)
				user.DELETE("/:id", userHandler.Delete)
			}

			mqttRepo := repository.NewMQTTConfigRepository(global.DB)
			mqttHandler := handler.NewMQTTHandler(mqttRepo)

			mqttGroup := protected.Group("/mqtt")
			{
				mqttGroup.GET("/config", mqttHandler.GetConfig)
				mqttGroup.PUT("/config", mqttHandler.UpdateConfig)
				mqttGroup.POST("/connect", mqttHandler.Connect)
				mqttGroup.POST("/disconnect", mqttHandler.Disconnect)
				mqttGroup.GET("/status", mqttHandler.GetStatus)
				mqttGroup.POST("/test", mqttHandler.TestConnection)
			}

			// device schema/template
			deviceTemplateHandler := handler.NewDeviceTemplateHandler()
			deviceTemplateGroup := protected.Group("/device")
			deviceTemplateGroup.GET("/template", deviceTemplateHandler.GetTemplate)

			deviceGroup := protected.Group("/device")
			{
				deviceGroup.GET("/list", deviceHandler.List)
				deviceGroup.GET("/:id", deviceHandler.Get)
				deviceGroup.GET("/:id/status", deviceHandler.GetStatus)
				deviceGroup.POST("", deviceHandler.Create)
				deviceGroup.PUT("/:id", deviceHandler.Update)
				deviceGroup.DELETE("/:id", deviceHandler.Delete)
				deviceGroup.POST("/:id/start", deviceHandler.Start)
				deviceGroup.POST("/:id/stop", deviceHandler.Stop)
			}
		}
	}

	return r
}
