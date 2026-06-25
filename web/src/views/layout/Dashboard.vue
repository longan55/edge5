<template>
  <div class="dashboard">
    <el-row :gutter="20">
      <el-col :span="6">
        <el-card class="stat-card">
          <div class="stat-content">
            <div class="stat-info">
              <div class="stat-title">设备总数</div>
              <div class="stat-value">{{ stats.deviceTotal }}</div>
            </div>
            <div class="stat-icon">
              <el-icon :size="40" color="#409EFF"><Box /></el-icon>
            </div>
          </div>
        </el-card>
      </el-col>

      <el-col :span="6">
        <el-card class="stat-card">
          <div class="stat-content">
            <div class="stat-info">
              <div class="stat-title">正常设备</div>
              <div class="stat-value">{{ stats.deviceOnline }}</div>
            </div>
          <div class="stat-action">
            <el-button
              type="primary"
              size="small"
              :disabled="isTesting"
              :loading="isTesting"
              @click="testDeviceConnections"
            >
              {{ isTesting ? '检测中...' : reloadButtonText }}
            </el-button>
          </div>
            <div class="stat-icon">
              <el-icon :size="40" color="#67C23A"><CircleCheck /></el-icon>
            </div>
          </div>
        </el-card>
      </el-col>

      <el-col :span="6">
        <el-card class="stat-card">
          <div class="stat-content">
            <div class="stat-info">
              <div class="stat-title">MQTT状态</div>
              <div class="stat-value">
                <el-tag :type="mqttConnected ? 'success' : 'danger'">
                  {{ mqttConnected ? '已连接' : '未连接' }}
                </el-tag>
              </div>
            </div>
            <div class="stat-icon">
              <el-icon :size="40" :color="mqttConnected ? '#67C23A' : '#F56C6C'"><Connection /></el-icon>
            </div>
          </div>
        </el-card>
      </el-col>

      <el-col :span="6">
        <el-card class="stat-card">
          <div class="stat-content">
            <div class="stat-info">
              <div class="stat-title">运行时间</div>
              <div class="stat-value">{{ uptime }}</div>
            </div>
            <div class="stat-icon">
              <el-icon :size="40" color="#E6A23C"><Timer /></el-icon>
            </div>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="20" style="margin-top: 20px">
      <el-col :span="8">
        <el-card>
          <template #header>
            <span>CPU 使用</span>
            <span style="float: right; font-size: 14px; color: #909399">{{ resources.cpuUsedPercent.toFixed(2) }}%</span>
          </template>
          <el-progress :percentage="resources.cpuUsedPercent" :color="getProgressColor(resources.cpuUsedPercent)" />
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card>
          <template #header>
            <span>内存使用</span>
            <span style="float: right; font-size: 14px; color: #909399">{{ formatBytes(resources.memUsed) }} / {{ formatBytes(resources.memTotal) }} ({{ resources.memUsedPercent.toFixed(2) }}%)</span>
          </template>
          <el-progress :percentage="resources.memUsedPercent" :color="getProgressColor(resources.memUsedPercent)" />
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card>
          <template #header>
            <span>磁盘使用</span>
            <span style="float: right; font-size: 14px; color: #909399">{{ formatBytes(resources.diskUsed) }} / {{ formatBytes(resources.diskTotal) }} ({{ resources.diskUsedPercent.toFixed(2) }}%)</span>
          </template>
          <el-progress :percentage="resources.diskUsedPercent" :color="getProgressColor(resources.diskUsedPercent)" />
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="20" style="margin-top: 20px">
      <el-col :span="12">
        <el-card>
          <template #header>
            <span>系统信息</span>
          </template>
          <el-descriptions :column="2" border>
            <el-descriptions-item label="网关SN">GWD001</el-descriptions-item>
            <el-descriptions-item label="程序版本">v1.0.0</el-descriptions-item>
            <el-descriptions-item label="数据库类型">SQLite3</el-descriptions-item>
            <el-descriptions-item label="最后更新">-</el-descriptions-item>
          </el-descriptions>
        </el-card>
      </el-col>

      <el-col :span="12">
        <el-card>
          <template #header>
            <span>最近操作</span>
          </template>
          <el-timeline>
            <el-timeline-item
              v-for="(activity, index) in activities"
              :key="index"
              :timestamp="activity.timestamp"
              placement="top"
            >
              {{ activity.content }}
            </el-timeline-item>
          </el-timeline>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import request from '@/utils/request'
import { useUserStore } from '@/stores/user'

const userStore = useUserStore()

const stats = ref({
  deviceTotal: 0,
  deviceOnline: 0
})

const mqttConnected = ref(false)
const uptime = ref('0天 0时 0分')
const isTesting = ref(false)
const reloadButtonText = ref('重新加载')
const lastTestTime = ref(0)
const cooldownTime = 30 // 30秒冷却时间

const activities = ref([
  { content: '系统启动', timestamp: new Date().toLocaleString() }
])

const resources = ref({
  cpuUsedPercent: 0,
  memTotal: 0,
  memUsed: 0,
  memUsedPercent: 0,
  diskTotal: 0,
  diskUsed: 0,
  diskUsedPercent: 0
})

let uptimeInterval = null
let isFetching = false

const fetchStats = async () => {
  if (isFetching) {
    return
  }
  isFetching = true
  try {
    const requests = [request.get('/device/list?page=1&page_size=1')]

    // 只有登录后才调用system/status接口
    if (userStore.token) {
      requests.push(request.get('/system/status'))
    }

    const responses = await Promise.all(requests)
    const deviceRes = responses[0]
    const systemRes = responses[1]

    stats.value.deviceTotal = deviceRes.data?.total || 0

    const deviceList = deviceRes.data?.list || []
    stats.value.deviceOnline = deviceList.filter(d => d.online).length

    if (systemRes) {
      mqttConnected.value = systemRes.data?.mqttConnected || false

      const uptimeStr = systemRes.data?.uptime || ''
      if (uptimeStr) {
        uptime.value = parseUptime(uptimeStr)
      }

      const resData = systemRes.data?.resources || {}
      resources.value = {
        cpuUsedPercent: resData.cpuUsedPercent || 0,
        memTotal: resData.memTotal || 0,
        memUsed: resData.memUsed || 0,
        memUsedPercent: resData.memUsedPercent || 0,
        diskTotal: resData.diskTotal || 0,
        diskUsed: resData.diskUsed || 0,
        diskUsedPercent: resData.diskUsedPercent || 0
      }
    }
  } catch (error) {
    console.error('获取统计数据失败:', error)
  } finally {
    isFetching = false
  }
}

// 解析 uptime 字符串，如 "up 0 days, 8 hours, 1 minute" -> "0天 8时 1分"
const parseUptime = (str) => {
  // 移除 "up " 前缀
  str = str.replace(/^up\s+/, '')

  const daysMatch = str.match(/(\d+)\s+days?/)
  const hoursMatch = str.match(/(\d+)\s+hours?/)
  const minutesMatch = str.match(/(\d+)\s+minutes?/)

  const days = daysMatch ? parseInt(daysMatch[1]) : 0
  const hours = hoursMatch ? parseInt(hoursMatch[1]) : 0
  const minutes = minutesMatch ? parseInt(minutesMatch[1]) : 0

  return `${days}天 ${hours}时 ${minutes}分`
}

const updateUptime = async () => {
  // 只更新 uptime，不重复调用 fetchStats
  // 只有登录后才调用system/status接口
  if (!userStore.token) {
    return
  }

  try {
    const systemRes = await request.get('/system/status')
    const uptimeStr = systemRes.data?.uptime || ''
    if (uptimeStr) {
      uptime.value = parseUptime(uptimeStr)
    }
  } catch (error) {
    console.error('获取运行时间失败:', error)
  }
}

const formatBytes = (bytes) => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

const getProgressColor = (percent) => {
  if (percent >= 90) return '#F56C6C'
  if (percent >= 70) return '#E6A23C'
  return '#67C23A'
}

// 测试设备连接
const testDeviceConnections = async () => {
  const now = Math.floor(Date.now() / 1000)
  const timeSinceLastTest = now - lastTestTime.value

  // 检查冷却时间
  if (timeSinceLastTest < cooldownTime && lastTestTime.value !== 0) {
    const remainingSeconds = cooldownTime - timeSinceLastTest
    return
  }

  isTesting.value = true
  try {
    await request.post('/device/test-connections')
    lastTestTime.value = now

    // 刷新设备列表
    await fetchStats()

    // 开始倒计时
    startCooldown()

    // 添加活动记录
    activities.value.unshift({
      content: '设备连接检测已完成',
      timestamp: new Date().toLocaleString()
    })
  } catch (error) {
    console.error('测试设备连接失败:', error)
    const errorMsg = error.response?.data?.message || '检测失败'
    // 添加错误活动记录
    activities.value.unshift({
      content: `设备连接检测失败: ${errorMsg}`,
      timestamp: new Date().toLocaleString()
    })
  } finally {
    isTesting.value = false
  }
}

// 开始倒计时
const startCooldown = () => {
  let remainingSeconds = cooldownTime
  const timer = setInterval(() => {
    remainingSeconds--
    reloadButtonText.value = `重新加载 (${remainingSeconds}s)`

    if (remainingSeconds <= 0) {
      clearInterval(timer)
      reloadButtonText.value = '重新加载'
    }
  }, 1000)
}

onMounted(() => {
  fetchStats()
  uptimeInterval = setInterval(fetchStats, 10000)
})

onUnmounted(() => {
  if (uptimeInterval) {
    clearInterval(uptimeInterval)
    uptimeInterval = null
  }
})
</script>

<style scoped>
.dashboard {
  padding: 20px;
}

.stat-card {
  cursor: pointer;
  transition: transform 0.3s;
}

.stat-card:hover {
  transform: translateY(-5px);
}

.stat-content {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.stat-action {
  margin-top: 10px;
  text-align: center;
}

.stat-title {
  font-size: 14px;
  color: #909399;
  margin-bottom: 10px;
}

.stat-value {
  font-size: 24px;
  font-weight: bold;
  color: #303133;
}
</style>
