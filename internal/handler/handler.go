package handler

import (
	"edge5/internal/model"
	"edge5/internal/service"
	"edge5/internal/utils/captcha"
	"edge5/internal/utils/response"
	"strconv"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	userService *service.UserService
}

func NewAuthHandler(userService *service.UserService) *AuthHandler {
	return &AuthHandler{userService: userService}
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req struct {
		Username  string `json:"username" binding:"required"`
		Password  string `json:"password" binding:"required"`
		CaptchaID string `json:"captcha_id" binding:"required"`
		Captcha   string `json:"captcha" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.CodeInvalidParam, "参数错误")
		return
	}

	if !captcha.VerifyCaptcha(req.CaptchaID, req.Captcha) {
		response.Error(c, response.CodeInvalidParam, "验证码错误")
		return
	}

	result, err := h.userService.Login(req.Username, req.Password, c.ClientIP())
	if err != nil {
		switch err {
		case service.ErrUserNotFound:
			response.Error(c, response.CodeInvalidUser, "用户不存在")
		case service.ErrInvalidPassword:
			response.Error(c, response.CodeInvalidPassword, "密码错误")
		case service.ErrUserDisabled:
			response.Error(c, response.CodeForbidden, "用户已被禁用")
		default:
			response.Error(c, response.CodeError, "登录失败")
		}
		return
	}

	response.Success(c, result)
}

func (h *AuthHandler) GetUserInfo(c *gin.Context) {
	userID := GetUserID(c)

	info, err := h.userService.GetUserInfo(userID)
	if err != nil {
		response.Error(c, response.CodeError, "获取用户信息失败")
		return
	}

	response.Success(c, info)
}

func (h *AuthHandler) GetCaptcha(c *gin.Context) {
	id, base64, err := captcha.GenerateCaptcha()
	if err != nil {
		response.Error(c, response.CodeError, "生成验证码失败")
		return
	}

	response.Success(c, gin.H{
		"captcha_id": id,
		"captcha":    base64,
	})
}

func GetUserID(c *gin.Context) uint64 {
	userID, exists := c.Get("user_id")
	if !exists {
		return 0
	}
	return userID.(uint64)
}

type UserHandler struct {
	userService *service.UserService
}

func NewUserHandler(userService *service.UserService) *UserHandler {
	return &UserHandler{userService: userService}
}

func (h *UserHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	users, total, err := h.userService.ListUsers(page, pageSize)
	if err != nil {
		response.Error(c, response.CodeError, "获取用户列表失败")
		return
	}

	response.Page(c, users, total)
}

func (h *UserHandler) Create(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
		Nickname string `json:"nickname"`
		Email    string `json:"email"`
		Phone    string `json:"phone"`
		RoleID   uint64 `json:"role_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.CodeInvalidParam, "参数错误")
		return
	}

	user := &model.User{
		Username: req.Username,
		Nickname: req.Nickname,
		Email:    req.Email,
		Phone:    req.Phone,
		RoleID:   req.RoleID,
		Status:   1,
	}

	if err := h.userService.CreateUser(user, req.Password); err != nil {
		response.Error(c, response.CodeError, "创建用户失败")
		return
	}

	response.Success(c, user)
}

func (h *UserHandler) Update(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)

	var req struct {
		Nickname string `json:"nickname"`
		Email    string `json:"email"`
		Phone    string `json:"phone"`
		RoleID   uint64 `json:"role_id"`
		Status   int8   `json:"status"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.CodeInvalidParam, "参数错误")
		return
	}

	user, err := h.userService.GetUserByID(id)
	if err != nil {
		response.Error(c, response.CodeError, "用户不存在")
		return
	}

	user.Nickname = req.Nickname
	user.Email = req.Email
	user.Phone = req.Phone
	user.RoleID = req.RoleID
	user.Status = req.Status

	if err := h.userService.UpdateUser(user); err != nil {
		response.Error(c, response.CodeError, "更新用户失败")
		return
	}

	response.Success(c, nil)
}

func (h *UserHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	userID := GetUserID(c)

	if id == userID {
		response.Error(c, response.CodeForbidden, "不能删除当前用户")
		return
	}

	if err := h.userService.DeleteUser(id); err != nil {
		response.Error(c, response.CodeError, "删除用户失败")
		return
	}

	response.Success(c, nil)
}

type DeviceHandler struct {
	service *service.DeviceService
}

func NewDeviceHandler(service *service.DeviceService) *DeviceHandler {
	return &DeviceHandler{service: service}
}

func (h *DeviceHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	deviceType := c.Query("device_type")
	brand := c.Query("brand")

	devices, total, err := h.service.ListDevices(page, pageSize, deviceType, brand)
	if err != nil {
		response.Error(c, response.CodeError, "获取设备列表失败")
		return
	}

	response.Page(c, devices, total)
}

func (h *DeviceHandler) Get(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)

	device, err := h.service.GetDevice(id)
	if err != nil {
		response.Error(c, response.CodeError, "获取设备失败")
		return
	}

	response.Success(c, device)
}

func (h *DeviceHandler) Create(c *gin.Context) {
	var req struct {
		DeviceSn   string `json:"device_sn" binding:"required"`
		DeviceName string `json:"device_name" binding:"required"`
		DeviceType string `json:"device_type" binding:"required"`
		Brand      string `json:"brand" binding:"required"`
		Protocol   string `json:"protocol" binding:"required"`
		Config     string `json:"config"`
		PluginName string `json:"plugin_name"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.CodeInvalidParam, "参数错误")
		return
	}

	device := &model.Device{
		DeviceSn:   req.DeviceSn,
		DeviceName: req.DeviceName,
		DeviceType: req.DeviceType,
		Brand:      req.Brand,
		Protocol:   req.Protocol,
		Config:     []byte(req.Config),
		PluginName: req.PluginName,
		Status:     1,
	}

	if err := h.service.CreateDevice(device); err != nil {
		response.Error(c, response.CodeError, "创建设备失败")
		return
	}

	response.Success(c, device)
}

func (h *DeviceHandler) Update(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)

	var req struct {
		DeviceName string `json:"device_name"`
		Config     string `json:"config"`
		Status     int8   `json:"status"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.CodeInvalidParam, "参数错误")
		return
	}

	device, err := h.service.GetDevice(id)
	if err != nil {
		response.Error(c, response.CodeError, "设备不存在")
		return
	}

	if req.DeviceName != "" {
		device.DeviceName = req.DeviceName
	}
	if req.Config != "" {
		device.Config = []byte(req.Config)
	}
	device.Status = req.Status

	if err := h.service.UpdateDevice(device); err != nil {
		response.Error(c, response.CodeError, "更新设备失败")
		return
	}

	response.Success(c, nil)
}

func (h *DeviceHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)

	if err := h.service.DeleteDevice(id); err != nil {
		response.Error(c, response.CodeError, "删除设备失败")
		return
	}

	response.Success(c, nil)
}

func (h *DeviceHandler) Start(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)

	if err := h.service.StartDevice(id); err != nil {
		response.Error(c, response.CodeError, "启动设备失败")
		return
	}

	response.Success(c, nil)
}

func (h *DeviceHandler) Stop(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)

	if err := h.service.StopDevice(id); err != nil {
		response.Error(c, response.CodeError, "停止设备失败")
		return
	}

	response.Success(c, nil)
}
