package sys

import (
	"edge5/internal/model"
	"time"
)

type User struct {
	model.BaseModel
	Username  string    `gorm:"uniqueIndex;size:64;not null" json:"username"`
	Password  string    `gorm:"size:128;not null" json:"-"`
	Nickname  string    `gorm:"size:64" json:"nickname"`
	Email     string    `gorm:"size:128" json:"email"`
	Phone     string    `gorm:"size:32" json:"phone"`
	Avatar    string    `gorm:"size:256" json:"avatar"`
	Status    int8      `gorm:"default:1;not null" json:"status"`
	RoleID    uint64    `gorm:"not null" json:"role_id"`
	Role      Role      `gorm:"foreignKey:RoleID" json:"role,omitempty"`
	LoginAt   time.Time `json:"login_at"`
	LoginIP   string    `gorm:"size:64" json:"login_ip"`
}

func (User) TableName() string {
	return "sys_user"
}

type Role struct {
	model.BaseModel
	Name    string `gorm:"size:64;not null" json:"name"`
	Code    string `gorm:"uniqueIndex;size:64;not null" json:"code"`
	Status  int8   `gorm:"default:1;not null" json:"status"`
	Sort    int    `gorm:"default:0" json:"sort"`
	Remark  string `gorm:"size:256" json:"remark"`
	Menus   []Menu `gorm:"many2many:sys_role_menu;" json:"menus,omitempty"`
}

func (Role) TableName() string {
	return "sys_role"
}

type Menu struct {
	model.BaseModel
	Name      string  `gorm:"size:64;not null" json:"name"`
	Path      string  `gorm:"size:128" json:"path"`
	Component string  `gorm:"size:256" json:"component"`
	Icon      string  `gorm:"size:64" json:"icon"`
	ParentID  uint64  `gorm:"default:0" json:"parent_id"`
	Sort      int     `gorm:"default:0" json:"sort"`
	Type      int8    `gorm:"default:1;not null" json:"type"`
	Status    int8    `gorm:"default:1;not null" json:"status"`
	Perms     string  `gorm:"size:128" json:"perms"`
	Children  []Menu  `gorm:"-" json:"children,omitempty"`
}

func (Menu) TableName() string {
	return "sys_menu"
}

type LoginLog struct {
	model.BaseModelWithoutTime
	UserID    uint64    `gorm:"not null" json:"user_id"`
	Username  string    `gorm:"size:64" json:"username"`
	IP        string    `gorm:"size:64" json:"ip"`
	Location  string    `gorm:"size:128" json:"location"`
	UserAgent string    `gorm:"size:512" json:"user_agent"`
	LoginAt   time.Time `json:"login_at"`
	Status    int8      `gorm:"default:1" json:"status"`
	Message   string    `gorm:"size:256" json:"message"`
}

func (LoginLog) TableName() string {
	return "sys_login_log"
}
