package service

import (
	"edge5/config"
	"edge5/internal/model"
	"edge5/internal/repository"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type UserService struct {
	userRepo *repository.UserRepository
	logger   *zap.Logger
}

func NewUserService(userRepo *repository.UserRepository, logger *zap.Logger) *UserService {
	return &UserService{userRepo: userRepo, logger: logger}
}

func (s *UserService) Login(username, password string, ip string) (*LoginResult, error) {
	user, err := s.userRepo.GetByUsername(username)
	if err != nil {
		return nil, ErrUserNotFound
	}

	if user.Status != 1 {
		return nil, ErrUserDisabled
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, ErrInvalidPassword
	}

	token, err := GenerateToken(user.ID, user.Username, "", user.Role.Code)
	if err != nil {
		return nil, err
	}

	user.LoginIP = ip
	s.userRepo.Update(user)

	return &LoginResult{
		Token: token,
		UserInfo: &UserInfo{
			ID:       user.ID,
			Username: user.Username,
			Nickname: user.Nickname,
			Avatar:   user.Avatar,
			Role: &RoleInfo{
				ID:   user.Role.ID,
				Name: user.Role.Name,
				Code: user.Role.Code,
			},
		},
	}, nil
}

func (s *UserService) GetUserInfo(userID uint64) (*UserInfo, error) {
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	return &UserInfo{
		ID:       user.ID,
		Username: user.Username,
		Nickname: user.Nickname,
		Avatar:   user.Avatar,
		Role: &RoleInfo{
			ID:   user.Role.ID,
			Name: user.Role.Name,
			Code: user.Role.Code,
		},
	}, nil
}

func (s *UserService) GetUserByID(id uint64) (*model.User, error) {
	return s.userRepo.GetByID(id)
}

func (s *UserService) CreateUser(user *model.User, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	user.Password = string(hash)
	return s.userRepo.Create(user)
}

func (s *UserService) Register(username, password, nickname, email, phone string, roleID uint64, ip string) (*model.User, error) {
	if username == "" {
		return nil, errors.New("username empty")
	}
	if password == "" {
		return nil, errors.New("password empty")
	}
	if roleID == 0 {
		roleID = 1
	}

	// 唯一性校验（用户名）
	_, err := s.userRepo.GetByUsername(username)
	if err == nil {
		return nil, ErrUserExists
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		// 数据库异常
		s.logger.Warn("注册校验用户是否存在失败", zap.Error(err))
		return nil, err
	}

	user := &model.User{
		Username: username,
		Password: "",
		Nickname: nickname,
		Email:    email,
		Phone:    phone,
		RoleID:   roleID,
		Status:   1,
		LoginIP:  ip,
	}

	if err := s.CreateUser(user, password); err != nil {
		return nil, err
	}

	return user, nil
}

func (s *UserService) UpdateUser(user *model.User) error {
	return s.userRepo.Update(user)
}

func (s *UserService) DeleteUser(id uint64) error {
	return s.userRepo.Delete(id)
}

func (s *UserService) ListUsers(page, pageSize int) ([]*model.User, int64, error) {
	return s.userRepo.List(page, pageSize)
}

// ChangePassword 修改密码
func (s *UserService) ChangePassword(userID uint64, oldPassword, newPassword string) error {
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return ErrUserNotFound
	}

	// 验证旧密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(oldPassword)); err != nil {
		return ErrInvalidPassword
	}

	// 新密码长度校验
	if len(newPassword) < 6 {
		return errors.New("密码至少6位")
	}

	// 生成新密码哈希
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user.Password = string(hash)
	return s.userRepo.Update(user)
}

// UpdateProfile 更新用户个人信息
func (s *UserService) UpdateProfile(userID uint64, nickname, email, phone string) error {
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return ErrUserNotFound
	}

	user.Nickname = nickname
	user.Email = email
	user.Phone = phone
	return s.userRepo.Update(user)
}

type LoginResult struct {
	Token    string    `json:"token"`
	UserInfo *UserInfo `json:"user"`
}

type UserInfo struct {
	ID       uint64    `json:"id"`
	Username string    `json:"username"`
	Nickname string    `json:"nickname"`
	Avatar   string    `json:"avatar"`
	Role     *RoleInfo `json:"role"`
}

type RoleInfo struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
	Code string `json:"code"`
}

var (
	ErrUserNotFound    = errors.New("用户不存在")
	ErrInvalidPassword = errors.New("密码错误")
	ErrUserDisabled    = errors.New("用户已被禁用")
	ErrUserExists      = errors.New("用户名已存在")
)

type DeviceService struct {
	deviceRepo       *repository.DeviceRepository
	deviceStatusRepo *repository.DeviceStatusRepository
}

func NewDeviceService(deviceRepo *repository.DeviceRepository, deviceStatusRepo *repository.DeviceStatusRepository) *DeviceService {
	return &DeviceService{
		deviceRepo:       deviceRepo,
		deviceStatusRepo: deviceStatusRepo,
	}
}

func (s *DeviceService) CreateDevice(device *model.Device) error {
	return s.deviceRepo.Create(device)
}

func (s *DeviceService) UpdateDevice(device *model.Device) error {
	return s.deviceRepo.Update(device)
}

func (s *DeviceService) DeleteDevice(id uint64) error {
	return s.deviceRepo.Delete(id)
}

func (s *DeviceService) GetDevice(id uint64) (*model.Device, error) {
	device, err := s.deviceRepo.GetByID(id)
	if err != nil {
		return nil, err
	}

	status, err := s.deviceStatusRepo.GetByDeviceID(device.ID)
	if err != nil {
		return nil, err
	}

	if status != nil {
		device.Online = status.Online
		device.LastHeartbeat = status.LastHeartbeat
		device.Message = status.Message
	} else {
		device.Online = false
		device.LastHeartbeat = time.Time{}
		device.Message = ""
	}

	return device, nil
}

func (s *DeviceService) ListDevices(page, pageSize int, deviceType, brand string) ([]*model.Device, int64, error) {
	devices, total, err := s.deviceRepo.List(page, pageSize, deviceType, brand)
	if err != nil {
		return nil, 0, err
	}

	// 填充在线状态（避免改表结构/复杂 SQL，先保证前端可用）
	for _, d := range devices {
		status, err := s.deviceStatusRepo.GetByDeviceID(d.ID)
		if err != nil {
			return nil, 0, err
		}
		if status != nil {
			d.Online = status.Online
			d.LastHeartbeat = status.LastHeartbeat
			d.Message = status.Message
		} else {
			d.Online = false
			d.LastHeartbeat = time.Time{}
			d.Message = ""
		}
	}

	return devices, total, nil
}

func (s *DeviceService) StartDevice(id uint64) error {
	device, err := s.deviceRepo.GetByID(id)
	if err != nil {
		return err
	}
	device.Status = 1
	if err := s.deviceRepo.Update(device); err != nil {
		return err
	}

	// 启动插件运行时（更新 device_status 在线/心跳/message）
	return deviceRuntime.Start(device, s.deviceStatusRepo)
}

func (s *DeviceService) StopDevice(id uint64) error {
	// 先停止插件运行时
	_ = deviceRuntime.Stop(id)

	device, err := s.deviceRepo.GetByID(id)
	if err != nil {
		return err
	}
	device.Status = 0
	return s.deviceRepo.Update(device)
}

func (s *DeviceService) GetDeviceStatus(deviceID uint64) (*model.DeviceStatus, error) {
	return s.deviceStatusRepo.GetByDeviceID(deviceID)
}

func GenerateToken(userID uint64, username, email, roleCode string) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"user_id":  userID,
		"username": username,
		"email":    email,
		"role":     roleCode,
		"iat":      now.Unix(),
		"exp":      now.Add(24 * time.Hour).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.CONFIG.JWT.Secret))
}
