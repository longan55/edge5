import request from '@/utils/request'

// 创建边缘计算任务
export function createEdgeTask(data) {
  return request.post('/edge', data)
}

// 更新边缘计算任务
export function updateEdgeTask(id, data) {
  return request.put(`/edge/${id}`, data)
}

// 删除边缘计算任务
export function deleteEdgeTask(id) {
  return request.delete(`/edge/${id}`)
}

// 批量删除
export function deleteEdgeTaskByIds(ids) {
  return request.post('/edge/batch-delete', { ids })
}

// 获取单个任务
export function getEdgeTask(id) {
  return request.get(`/edge/${id}`)
}

// 分页查询
export function getEdgeTasks(params) {
  return request.get('/edge/list', { params })
}

// 启动 / 停止
export function startEdgeTask(id) {
  return request.post(`/edge/${id}/start`)
}
export function stopEdgeTask(id) {
  return request.post(`/edge/${id}/stop`)
}

// 最近上报数据
export function getEdgeTaskData(id) {
  return request.get(`/edge/${id}/data`)
}

// 支持的算子及参数元信息
export function getEdgeFunctions() {
  return request.get('/edge/functions')
}

// 在线分析某任务下指定规则（如 counter_analog 模拟信号计数的峰谷学习）。
// 任务必须先启动，因为分析数据在算子内存。
export function analyzeEdgeRule(id, ruleName) {
  return request.get(`/edge/${id}/analyze`, { params: { ruleName } })
}

// 设备列表（规则输入点位所属设备）
export function getDevices(params) {
  return request.get('/device/list', { params })
}

// 设备协议的采集参数 Schema（用于解析类型选项）
export function getReadParamsSchema(deviceId) {
  return request.get(`/task/device-read-params-schema/${deviceId}`)
}
