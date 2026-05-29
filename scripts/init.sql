-- Edge5 数据库初始化脚本

-- 用户表
CREATE TABLE IF NOT EXISTS sys_user (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username VARCHAR(64) NOT NULL UNIQUE,
    password VARCHAR(128) NOT NULL,
    nickname VARCHAR(64),
    email VARCHAR(128),
    phone VARCHAR(32),
    avatar VARCHAR(256),
    status TINYINT DEFAULT 1 NOT NULL,
    role_id INTEGER NOT NULL,
    login_at DATETIME,
    login_ip VARCHAR(64),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 角色表
CREATE TABLE IF NOT EXISTS sys_role (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name VARCHAR(64) NOT NULL,
    code VARCHAR(64) NOT NULL UNIQUE,
    status TINYINT DEFAULT 1 NOT NULL,
    sort INTEGER DEFAULT 0,
    remark VARCHAR(256),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 菜单表
CREATE TABLE IF NOT EXISTS sys_menu (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name VARCHAR(64) NOT NULL,
    path VARCHAR(128),
    component VARCHAR(256),
    icon VARCHAR(64),
    parent_id INTEGER DEFAULT 0,
    sort INTEGER DEFAULT 0,
    type TINYINT DEFAULT 1 NOT NULL,
    status TINYINT DEFAULT 1 NOT NULL,
    perms VARCHAR(128),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 角色菜单关联表
CREATE TABLE IF NOT EXISTS sys_role_menu (
    role_id INTEGER NOT NULL,
    menu_id INTEGER NOT NULL,
    PRIMARY KEY (role_id, menu_id)
);

-- 登录日志表
CREATE TABLE IF NOT EXISTS sys_login_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    username VARCHAR(64),
    ip VARCHAR(64),
    location VARCHAR(128),
    user_agent VARCHAR(512),
    login_at DATETIME,
    status TINYINT DEFAULT 1,
    message VARCHAR(256)
);

-- MQTT配置表
CREATE TABLE IF NOT EXISTS mqtt_config (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    broker VARCHAR(256) NOT NULL,
    port INTEGER NOT NULL,
    username VARCHAR(64),
    password VARCHAR(128),
    client_id VARCHAR(64) NOT NULL,
    keep_alive INTEGER DEFAULT 60,
    qos TINYINT DEFAULT 1,
    on TINYINT DEFAULT 0,
    gateway_sn VARCHAR(64) UNIQUE NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 设备表
CREATE TABLE IF NOT EXISTS device (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_sn VARCHAR(64) NOT NULL UNIQUE,
    device_name VARCHAR(128) NOT NULL,
    device_type VARCHAR(32) NOT NULL,
    brand VARCHAR(32) NOT NULL,
    protocol VARCHAR(32) NOT NULL,
    status TINYINT DEFAULT 1,
    config TEXT,
    plugin_name VARCHAR(64),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 设备状态表
CREATE TABLE IF NOT EXISTS device_status (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id INTEGER NOT NULL UNIQUE,
    online BOOLEAN DEFAULT FALSE,
    last_heartbeat DATETIME,
    message VARCHAR(512)
);

-- 初始化管理员角色
INSERT OR IGNORE INTO sys_role (id, name, code, status, sort) VALUES
(1, '超级管理员', 'admin', 1, 1);

-- 初始化默认管理员用户 (密码: admin123)
-- 密码使用 bcrypt 加密
INSERT OR IGNORE INTO sys_user (id, username, password, nickname, role_id, status) VALUES
(1, 'admin', '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZRGdjGj/n3.RsikMnF.D3p3m3w3eO', '管理员', 1, 1);

-- 初始化基础菜单
INSERT OR IGNORE INTO sys_menu (id, name, path, component, icon, parent_id, sort, type, status) VALUES
(1, '系统管理', '/system', 'Layout', 'Setting', 0, 1, 1, 1),
(2, '用户管理', 'user', '/system/user/index', 'User', 1, 1, 2, 1),
(3, '角色管理', 'role', '/system/role/index', 'UserFilled', 1, 2, 2, 1),
(4, '菜单管理', 'menu', '/system/menu/index', 'Menu', 1, 3, 2, 1),
(5, 'MQTT配置', '/mqtt', 'Layout', 'Connection', 0, 2, 1, 1),
(6, 'MQTT设置', 'config', '/mqtt/config/index', 'Setting', 5, 1, 2, 1),
(7, '设备管理', '/device', 'Layout', 'Box', 0, 3, 1, 1),
(8, '设备列表', 'list', '/device/list/index', 'List', 7, 1, 2, 1);

-- 初始化角色菜单关联
INSERT OR IGNORE INTO sys_role_menu (role_id, menu_id) VALUES
(1, 1), (1, 2), (1, 3), (1, 4), (1, 5), (1, 6), (1, 7), (1, 8);
