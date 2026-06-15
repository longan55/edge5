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

const stats = ref({
  deviceTotal: 0,
  deviceOnline: 0
})

const mqttConnected = ref(false)
const uptime = ref('0天 0时 0分')

const activities = ref([
  { content: '系统启动', timestamp: new Date().toLocaleString() }
])

let uptimeInterval = null

const fetchStats = async () => {
  try {
    const [deviceRes, mqttRes] = await Promise.all([
      request.get('/device/list?page=1&page_size=1'),
      request.get('/mqtt/status')
    ])

    stats.value.deviceTotal = deviceRes.data?.total || 0

    const deviceList = deviceRes.data?.list || []
    stats.value.deviceOnline = deviceList.filter(d => d.online).length

    mqttConnected.value = mqttRes.data?.connected || false

    // 解析后端返回的 uptime 字符串，格式如 "up 0 days, 8 hours, 1 minute"
    const uptimeStr = mqttRes.data?.uptime || ''
    if (uptimeStr) {
      uptime.value = parseUptime(uptimeStr)
    }
  } catch (error) {
    console.error('获取统计数据失败:', error)
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

const updateUptime = () => {
  // 从后端获取 uptime，不再本地计算
  fetchStats()
}

onMounted(() => {
  fetchStats()
  setInterval(fetchStats, 30000)

  updateUptime()
  uptimeInterval = setInterval(updateUptime, 60000)
})

onUnmounted(() => {
  if (uptimeInterval) {
    clearInterval(uptimeInterval)
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
