package handler

import (
	"fmt"
	"strconv"

	"edge5/global"
	"edge5/internal/model"
	"edge5/internal/pkg/protocol"
	"edge5/internal/repository"
	"edge5/internal/service"
	"edge5/internal/utils/response"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type TaskHandler struct {
	svc *service.TaskService
}

func NewTaskHandler(dbLogger *zap.Logger) *TaskHandler {
	repo := repository.NewTaskRepository(global.DB, dbLogger)
	svc := service.NewTaskService(repo, dbLogger)
	// 自动建表
	if err := repo.AutoMigrate(); err != nil {
		dbLogger.Error("Task 自动建表失败", zap.Error(err))
	}
	return &TaskHandler{svc: svc}
}

// CreateTask 创建任务
func (h *TaskHandler) CreateTask(c *gin.Context) {
	var task model.Task
	if err := c.ShouldBindJSON(&task); err != nil {
		response.Error(c, response.CodeInvalidParam, "参数错误: "+err.Error())
		return
	}
	if err := h.svc.Create(&task); err != nil {
		response.Error(c, response.CodeServerError, "创建失败: "+err.Error())
		return
	}
	response.Success(c, task)
}

// UpdateTask 更新任务
func (h *TaskHandler) UpdateTask(c *gin.Context) {
	var task model.Task
	if err := c.ShouldBindJSON(&task); err != nil {
		response.Error(c, response.CodeInvalidParam, "参数错误: "+err.Error())
		return
	}
	if err := h.svc.Update(&task); err != nil {
		response.Error(c, response.CodeServerError, "更新失败: "+err.Error())
		return
	}
	response.Success(c, task)
}

// DeleteTask 删除单个任务
func (h *TaskHandler) DeleteTask(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		response.Error(c, response.CodeInvalidParam, "无效的 ID")
		return
	}
	if err := h.svc.Delete(id); err != nil {
		response.Error(c, response.CodeServerError, "删除失败: "+err.Error())
		return
	}
	response.Success(c, nil)
}

// DeleteTaskBatch 批量删除任务
func (h *TaskHandler) DeleteTaskBatch(c *gin.Context) {
	var req struct {
		IDs []uint64 `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.CodeInvalidParam, "参数错误")
		return
	}
	if err := h.svc.DeleteBatch(req.IDs); err != nil {
		response.Error(c, response.CodeServerError, "批量删除失败: "+err.Error())
		return
	}
	response.Success(c, nil)
}

// GetTask 获取单个任务
func (h *TaskHandler) GetTask(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		response.Error(c, response.CodeInvalidParam, "无效的 ID")
		return
	}
	task, err := h.svc.GetByID(id)
	if err != nil {
		response.Error(c, response.CodeNotFound, "查询失败: "+err.Error())
		return
	}
	response.Success(c, task)
}

// ListTasks 分页查询任务列表
func (h *TaskHandler) ListTasks(c *gin.Context) {
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("pageSize", "10")
	name := c.Query("name")

	page, _ := strconv.Atoi(pageStr)
	pageSize, _ := strconv.Atoi(pageSizeStr)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	tasks, total, err := h.svc.List(page, pageSize, name)
	if err != nil {
		response.Error(c, response.CodeServerError, "查询失败: "+err.Error())
		return
	}

	response.Success(c, gin.H{
		"tasks":    tasks,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

// StartTask 开启任务
func (h *TaskHandler) StartTask(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		response.Error(c, response.CodeInvalidParam, "无效的 ID")
		return
	}
	if err := h.svc.StartTask(id); err != nil {
		response.Error(c, response.CodeServerError, "开启失败: "+err.Error())
		return
	}
	response.Success(c, nil)
}

// StopTask 关闭任务
func (h *TaskHandler) StopTask(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		response.Error(c, response.CodeInvalidParam, "无效的 ID")
		return
	}
	if err := h.svc.StopTask(id); err != nil {
		response.Error(c, response.CodeServerError, "关闭失败: "+err.Error())
		return
	}
	response.Success(c, nil)
}

// getDeviceProtocolName 从设备记录中获取协议名称
func getDeviceProtocolName(deviceID uint64) (string, error) {
	var device model.Device
	if err := global.DB.First(&device, deviceID).Error; err != nil {
		return "", fmt.Errorf("设备不存在")
	}
	if device.Protocol != "" {
		return device.Protocol, nil
	}
	return "", fmt.Errorf("设备未绑定协议")
}

// GetReadParamsSchema 获取指定协议的采集参数 Schema
func (h *TaskHandler) GetReadParamsSchema(c *gin.Context) {
	deviceIDStr := c.Param("deviceId")
	deviceID, err := strconv.ParseUint(deviceIDStr, 10, 64)
	if err != nil {
		response.Error(c, response.CodeInvalidParam, "无效的设备 ID")
		return
	}

	protocolName, err := getDeviceProtocolName(deviceID)
	if err != nil {
		response.Error(c, response.CodeNotFound, err.Error())
		return
	}

	// 从协议注册表中获取协议信息
	reg := protocol.DefaultRegistry()
	infos := reg.List()
	var schema []protocol.ReadParamSchema
	for _, info := range infos {
		if protocol.GetInfoString(info, "name") == protocolName {
			schema = protocol.ExtractReadParamsSchema(info)
			break
		}
	}

	if schema == nil {
		// 返回默认 schema（通用参数）
		schema = []protocol.ReadParamSchema{
			{Name: "address", CName: "地址", Type: "string"},
			{Name: "offset", CName: "偏移量", Type: "int"},
			{Name: "parseType", CName: "解析类型", Type: "select", Choices: []string{"bool", "short", "ushort", "int", "uint", "long", "ulong", "float", "double", "string"}},
		}
	}

	response.Success(c, gin.H{
		"deviceId":         deviceID,
		"protocol":         protocolName,
		"readParamsSchema": schema,
	})
}
