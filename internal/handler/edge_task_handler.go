package handler

import (
	"fmt"
	"strconv"
	"strings"

	"edge5/global"
	"edge5/internal/core/expr"
	"edge5/internal/model"
	"edge5/internal/repository"
	"edge5/internal/service"
	"edge5/internal/utils/response"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type EdgeTaskHandler struct {
	svc *service.EdgeTaskService
}

func NewEdgeTaskHandler(dbLogger *zap.Logger) *EdgeTaskHandler {
	repo := repository.NewEdgeTaskRepository(global.DB, dbLogger)
	deviceRepo := repository.NewDeviceRepository(global.DB)
	service.NewEdgeTaskScheduler(repo, deviceRepo, dbLogger)
	svc := service.NewEdgeTaskService(repo, deviceRepo, dbLogger)
	if err := repo.AutoMigrate(); err != nil {
		dbLogger.Error("EdgeTask 自动建表失败", zap.Error(err))
	}
	return &EdgeTaskHandler{svc: svc}
}

func (h *EdgeTaskHandler) Create(c *gin.Context) {
	var task model.EdgeTask
	if err := c.ShouldBindJSON(&task); err != nil {
		response.Error(c, response.CodeInvalidParam, "参数错误: "+err.Error())
		return
	}
	if err := validateTask(&task); err != nil {
		response.Error(c, response.CodeInvalidParam, err.Error())
		return
	}
	if err := h.svc.Create(&task); err != nil {
		response.Error(c, response.CodeServerError, "创建失败: "+err.Error())
		return
	}
	response.Success(c, task)
}

func (h *EdgeTaskHandler) Update(c *gin.Context) {
	var task model.EdgeTask
	if err := c.ShouldBindJSON(&task); err != nil {
		response.Error(c, response.CodeInvalidParam, "参数错误: "+err.Error())
		return
	}
	if err := validateTask(&task); err != nil {
		response.Error(c, response.CodeInvalidParam, err.Error())
		return
	}
	if err := h.svc.Update(&task); err != nil {
		response.Error(c, response.CodeServerError, "更新失败: "+err.Error())
		return
	}
	response.Success(c, task)
}

func (h *EdgeTaskHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
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

func (h *EdgeTaskHandler) DeleteBatch(c *gin.Context) {
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

func (h *EdgeTaskHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
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

func (h *EdgeTaskHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	name := c.Query("name")
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

	scheduler := service.GetEdgeTaskScheduler()
	result := make([]map[string]interface{}, len(tasks))
	for i, t := range tasks {
		status := "stopped"
		if scheduler != nil {
			status = string(scheduler.GetTaskStatus(t.ID))
		}
		result[i] = map[string]interface{}{
			"id":             t.ID,
			"name":           t.Name,
			"deviceId":       t.DeviceID,
			"upTopic":        t.UpTopic,
			"readInterval":   t.ReadInterval, // 兼容老前端
			"sampleInterval": t.SampleInterval,
			"reportInterval": t.ReportInterval,
			"inputs":         t.Inputs,
			"rules":          t.Rules,
			"state":          t.State,
			"status":         status,
			"createdAt":      t.CreatedAt,
			"updatedAt":      t.UpdatedAt,
		}
	}

	response.Success(c, gin.H{
		"tasks":    result,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

func (h *EdgeTaskHandler) Start(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
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

func (h *EdgeTaskHandler) Stop(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
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

func (h *EdgeTaskHandler) Data(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, response.CodeInvalidParam, "无效的 ID")
		return
	}
	data, err := h.svc.GetTaskData(id)
	if err != nil {
		response.Error(c, response.CodeServerError, "查询失败: "+err.Error())
		return
	}
	response.Success(c, data)
}

// Functions 返回支持的算子及其参数元信息
func (h *EdgeTaskHandler) Functions(c *gin.Context) {
	response.Success(c, h.svc.ListFunctions())
}

// Analyze 手动触发某任务下指定规则的在线分析（如 counter_ac 的峰谷学习）。
// GET /edge/:id/analyze?ruleName=xxx
// 任务必须在运行，因为分析数据（滑动窗口）在算子内存中。
func (h *EdgeTaskHandler) Analyze(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, response.CodeInvalidParam, "无效的任务 ID")
		return
	}
	ruleName := c.Query("ruleName")
	if ruleName == "" {
		response.Error(c, response.CodeInvalidParam, "缺少 ruleName 参数")
		return
	}
	info, err := h.svc.AnalyzeRule(id, ruleName)
	if err != nil {
		response.Error(c, response.CodeServerError, err.Error())
		return
	}
	response.Success(c, info)
}

// validateRules 做最基本的入参校验（深层校验由引擎在启动时完成）
// validateTask 校验点位池与规则的引用合法性（深层校验由引擎在启动时完成）
func validateTask(task *model.EdgeTask) error {
	// 点位池校验：Var 唯一、地址非空、表达式合法（仅允许变量 x）
	poolVars := make(map[string]bool)
	for i, in := range task.Inputs {
		if in.Var == "" {
			return errResp("点位池第 %d 个点位缺少变量名", i+1)
		}
		if in.Address == "" {
			return errResp("点位池变量 %s 缺少地址", in.Var)
		}
		if poolVars[in.Var] {
			return errResp("点位池变量名重复: %s", in.Var)
		}
		poolVars[in.Var] = true
		if src := strings.TrimSpace(in.Expr); src != "" {
			if _, err := expr.CompileAllowed(src, map[string]bool{"x": true}); err != nil {
				return errResp("点位池变量 %s 的计算表达式无效: %v", in.Var, err)
			}
		}
	}

	// 规则校验：输出名唯一、有算子类型、引用的 Var 全部在点位池中
	if len(task.Rules) == 0 {
		return errResp("至少配置一条计算规则")
	}
	names := make(map[string]bool)
	for i, r := range task.Rules {
		if r.Name == "" {
			return errResp("第 %d 条规则缺少输出名称", i+1)
		}
		if names[r.Name] {
			return errResp("规则输出名称重复: %s", r.Name)
		}
		names[r.Name] = true
		if r.FuncType == "" {
			return errResp("规则 %s 缺少算子类型", r.Name)
		}
		if len(r.Vars) == 0 {
			return errResp("规则 %s 至少需要引用一个变量", r.Name)
		}
		vars := make(map[string]bool)
		for j, vr := range r.Vars {
			if vr.Var == "" {
				return errResp("规则 %s 第 %d 个引用缺少变量名", r.Name, j+1)
			}
			if !poolVars[vr.Var] {
				return errResp("规则 %s 引用了未在点位池中定义的变量 %s", r.Name, vr.Var)
			}
			if vars[vr.Var] {
				return errResp("规则 %s 内引用变量重复: %s", r.Name, vr.Var)
			}
			vars[vr.Var] = true
		}
	}
	return nil
}

func errResp(format string, args ...interface{}) error {
	return fmt.Errorf(format, args...)
}
