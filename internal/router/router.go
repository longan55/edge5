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

	// 系统状态接口（无需认证，供仪表盘首页定时调用）
	systemHandler := handler.NewSystemHandler()
	r.GET("/api/system/status", systemHandler.GetStatus)

	// MQTT 状态接口（无需认证，供仪表盘首页定时调用，向后兼容）
	mqttRepoForStatus := repository.NewMQTTConfigRepository(global.DB)
	mqttHandlerForStatus := handler.NewMQTTHandler(mqttRepoForStatus)
	r.GET("/api/mqtt/status", mqttHandlerForStatus.GetStatus)

	api := r.Group("/api")
	{
		api.POST("/login", authHandler.Login)
		api.POST("/register", authHandler.Register)

		protected := api.Group("")
		protected.Use(middleware.JWTAuth())
		{
			protected.GET("/user/info", authHandler.GetUserInfo)
			protected.GET("/user/detail", authHandler.GetUserDetail)
			protected.PUT("/user/profile", authHandler.UpdateProfile)
			protected.PUT("/user/password", authHandler.ChangePassword)

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
				// mqttGroup.GET("/status", mqttHandler.GetStatus) // 已移到外部，无需认证
				mqttGroup.POST("/test", mqttHandler.TestConnection)

				// 主题模板配置
				mqttGroup.GET("/topics", mqttHandler.GetTopics)
				mqttGroup.PUT("/topics", mqttHandler.BatchSaveTopics)
				mqttGroup.POST("/topics/reset", mqttHandler.ResetTopics)
				// 全局主题配置
				mqttGroup.GET("/topic-config", mqttHandler.GetTopicConfig)
				mqttGroup.PUT("/topic-config", mqttHandler.SaveTopicConfig)
				mqttGroup.POST("/topic-config/reset", mqttHandler.ResetTopicConfig)
				// 热更新主题配置
				mqttGroup.POST("/topic-config/reload", mqttHandler.ReloadTopicConfig)
			}

			// device add options (deviceTypes + protocolOptions)
			deviceOptionsHandler := handler.NewDeviceOptionsHandler()
			deviceOptionsGroup := protected.Group("/device")
			deviceOptionsGroup.GET("/options", deviceOptionsHandler.GetDeviceOptions)

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
				deviceGroup.POST("/test-connections", handler.NewDeviceInitHandler().TestConnections)
			}

			// 采集任务
			taskHandler := handler.NewTaskHandler(global.Logger)
			taskGroup := protected.Group("/task")
			{
				taskGroup.GET("/list", taskHandler.ListTasks)
				taskGroup.GET("/:id", taskHandler.GetTask)
				taskGroup.GET("/:id/data", taskHandler.GetTaskData)
				taskGroup.POST("", taskHandler.CreateTask)
				taskGroup.PUT("/:id", taskHandler.UpdateTask)
				taskGroup.DELETE("/:id", taskHandler.DeleteTask)
				taskGroup.POST("/batch-delete", taskHandler.DeleteTaskBatch)
				taskGroup.POST("/:id/start", taskHandler.StartTask)
				taskGroup.POST("/:id/stop", taskHandler.StopTask)
			}

			// 设备调试
			debugHandler := handler.NewDeviceDebugHandler(global.Logger)
			deviceDebugGroup := protected.Group("/device")
			{
				deviceDebugGroup.GET("/:id/debug/info", debugHandler.GetDeviceDebugInfo)
				deviceDebugGroup.POST("/:id/debug/read", debugHandler.DebugRead)
				deviceDebugGroup.POST("/:id/debug/write", debugHandler.DebugWrite)
			}

			// 获取设备协议的采集参数 Schema（使用固定前缀避免与 :id 冲突）
			protected.GET("/task/device-read-params-schema/:deviceId", taskHandler.GetReadParamsSchema)
		}
	}

	return r
}
