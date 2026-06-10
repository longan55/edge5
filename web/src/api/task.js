import request from '@/utils/request'

// 创建任务
export function createTask(data) {
  return request.post('/task', data)
}

// 更新任务
export function updateTask(id, data) {
  return request.put(`/task/${id}`, data)
}

// 删除任务
export function deleteTask(id) {
  return request.delete(`/task/${id}`)
}

// 批量删除
export function deleteTaskByIds(ids) {
  return request.post('/task/batch-delete', { ids })
}

// 获取单个任务
export function getTask(id) {
  return request.get(`/task/${id}`)
}

// 分页查询任务列表
export function getTasks(params) {
  return request.get('/task/list', { params })
}

// 开启任务
export function startTask(id) {
  return request.post(`/task/${id}/start`)
}

// 关闭任务
export function stopTask(id) {
  return request.post(`/task/${id}/stop`)
}

// 获取设备列表（用于任务关联设备）
export function getDevices(params) {
  return request.get('/device/list', { params })
}

// 获取设备协议的采集参数 Schema
export function getReadParamsSchema(deviceId) {
  return request.get(`/task/device-read-params-schema/${deviceId}`)
}
