import request from '@/utils/request'

// 获取设备调试信息（是否支持调试、采集参数 schema）
export function getDeviceDebugInfo(deviceId) {
  return request.get(`/device/${deviceId}/debug/info`)
}

// 调试读取
export function debugRead(deviceId, params) {
  return request.post(`/device/${deviceId}/debug/read`, { params })
}

// 调试写入
export function debugWrite(deviceId, params) {
  return request.post(`/device/${deviceId}/debug/write`, { params })
}

// 获取设备选项（包含 supportDebug 标识）
export function getDeviceOptions() {
  return request.get('/device/options')
}
