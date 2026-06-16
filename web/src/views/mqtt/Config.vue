<template>
  <div class="mqtt-config">
    <el-card>
      <template #header>
        <span>MQTT 配置</span>
      </template>

      <!-- 基础配置 -->
      <div class="config-section">
        <div class="section-header">
          <h4>基础</h4>
        </div>

        <el-form :model="form" label-width="120px">
          <!-- 主机 -->
          <el-form-item label="主机*">
            <div class="host-input-group">
              <el-select v-model="form.protocol" class="protocol-select">
                <el-option label="mqtt://" value="mqtt://" />
                <el-option label="mqtts://" value="mqtts://" />
              </el-select>
              <el-input v-model="form.host" placeholder="输入 IP 地址" />
            </div>
          </el-form-item>

          <!-- 端口 -->
          <el-form-item label="端口*">
            <el-input-number v-model="form.port" :min="1" :max="65535" />
          </el-form-item>

          <!-- Client ID -->
          <el-form-item label="Client ID">
            <div class="client-id-group">
              <el-input v-model="form.client_id" placeholder="输入 Client ID" />
              <el-button @click="generateClientId" icon="Refresh">生成</el-button>
            </div>
          </el-form-item>

          <!-- 用户名 -->
          <el-form-item label="用户名">
            <el-input v-model="form.username" placeholder="输入用户名" />
          </el-form-item>

          <!-- 密码 -->
          <el-form-item label="密码">
            <el-input v-model="form.password" type="password" show-password placeholder="输入密码" />
          </el-form-item>

          <!-- SSL/TLS 开关 -->
          <el-form-item label="SSL/TLS">
            <el-switch v-model="form.ssl" @change="onSSLChange" />
          </el-form-item>

          <!-- SSL 相关配置（根据 SSL 开关显示/隐藏） -->
          <div v-show="form.ssl" class="ssl-content" @mousedown.stop>
            <el-form-item label="SSL Secure">
              <el-switch v-model="form.ssl_verify" />
            </el-form-item>

            <el-form-item label="ALPN标签">
              <el-input v-model="form.alpn_tag" placeholder="输入 ALPN 标签" />
            </el-form-item>

            <el-form-item label="Certificate">
              <el-radio-group v-model="form.cert_type">
                <el-radio label="ca_signed">CA已签署服务器证书</el-radio>
                <el-radio label="self_signed">CA自签名证书文件</el-radio>
              </el-radio-group>
            </el-form-item>

            <!-- 自签名证书文件 -->
            <div v-show="form.cert_type === 'self_signed'" class="cert-content">
              <el-form-item label="CA文件">
                <div class="file-input-group">
                  <el-input v-model="form.ca_file" readonly placeholder="选择 CA 文件" />
                  <el-button @click="selectFile('ca_file')">选择文件</el-button>
                </div>
              </el-form-item>

              <el-form-item label="客户端证书文件">
                <div class="file-input-group">
                  <el-input v-model="form.cert_file" readonly placeholder="选择证书文件" />
                  <el-button @click="selectFile('cert_file')">选择文件</el-button>
                </div>
              </el-form-item>

              <el-form-item label="客户端秘钥文件">
                <div class="file-input-group">
                  <el-input v-model="form.key_file" readonly placeholder="选择秘钥文件" />
                  <el-button @click="selectFile('key_file')">选择文件</el-button>
                </div>
              </el-form-item>
            </div>
          </div>
        </el-form>
      </div>

      <!-- 高级配置 -->
      <div class="config-section">
        <div class="section-header" @click="toggleAdvanced">
          <h4>高级</h4>
          <!-- <el-icon>{{ advancedExpanded ? 'ArrowUp' : 'ArrowDown' }}</el-icon> -->
        </div>

        <el-transition name="slide">
          <div v-show="advancedExpanded" class="advanced-content">
            <el-form :model="form" label-width="120px">
              <el-form-item label="MQTT版本">
                <el-select v-model="form.version" class="version-select">
                  <el-option label="5.0" value="5.0" />
                </el-select>
              </el-form-item>

              <el-form-item label="连接超时(秒)">
                <el-input-number v-model="form.connect_timeout" :min="1" :max="60" />
              </el-form-item>

              <el-form-item label="保持活跃(秒)">
                <el-input-number v-model="form.keep_alive" :min="10" :max="300" />
              </el-form-item>

              <el-form-item label="自动重连">
                <el-switch v-model="form.auto_reconnect" />
              </el-form-item>

              <el-form-item label="重连周期(ms)">
                <el-input-number v-model="form.reconnect_period" :min="1000" :max="60000" />
              </el-form-item>

              <el-form-item label="全新开始">
                <el-switch v-model="form.clean_start" @change="onCleanStartChange" />
              </el-form-item>

              <el-form-item label="会话过期间隔(秒)">
                <el-input-number v-model="form.session_expiry" :min="0" :max="86400" />
              </el-form-item>

              <el-form-item label="接收最大值">
                <el-input-number 
                  v-model="form.receive_max" 
                  :min="0" 
                  :step="1"
                  @change="onReceiveMaxChange"
                />
              </el-form-item>

              <el-form-item label="最大数据包">
                <el-input-number 
                  v-model="form.max_packet_size" 
                  :min="0" 
                  :step="1"
                  @change="onMaxPacketSizeChange"
                />
              </el-form-item>

              <el-form-item label="主题别名最大值">
                <el-input-number 
                  v-model="form.topic_alias_max" 
                  :min="0" 
                  :step="1"
                  @change="onTopicAliasMaxChange"
                />
              </el-form-item>

              <el-form-item label="请求响应信息">
                <el-switch v-model="form.request_response_info" />
              </el-form-item>

              <el-form-item label="请求问题信息">
                <el-switch v-model="form.request_problem_info" />
              </el-form-item>
            </el-form>
          </div>
        </el-transition>
      </div>

      <!-- 操作按钮 -->
      <div class="action-buttons">
        <el-button type="primary" @click="handleSave">保存配置</el-button>
        <el-button :type="mqttConnected ? 'danger' : 'success'" @click="handleConnect">
          {{ mqttConnected ? '断开连接' : '连接' }}
        </el-button>
      </div>

      <el-divider />

      <!-- 状态信息 -->
      <div class="status-info">
        <el-tag :type="mqttConnected ? 'success' : 'danger'" size="large">
          {{ mqttConnected ? '已连接' : '未连接' }}
        </el-tag>
        <span style="margin-left: 20px">缓存队列: {{ cacheSize }} 条消息</span>
      </div>
    </el-card>

    <!-- 主题配置 -->
    <el-card style="margin-top: 20px;">
      <template #header>
        <div class="topic-header">
          <span>主题配置</span>
          <div>
            <el-button type="primary" size="small" @click="handleSaveTopics">保存主题</el-button>
            <el-button size="small" @click="handleResetTopics">恢复默认</el-button>
          </div>
        </div>
      </template>

      <div class="topic-global">
        <el-form :inline="true" :model="topicConfig" class="topic-global-form">
          <el-form-item label="前缀">
            <el-input v-model="topicConfig.prefix" placeholder="/aixot" style="width: 150px" />
          </el-form-item>
          <el-form-item label="区分上下行">
            <el-switch v-model="topicConfig.show_direction" />
          </el-form-item>
          <el-form-item label="上行关键词">
            <el-input v-model="topicConfig.up_keyword" placeholder="up" style="width: 100px" />
          </el-form-item>
          <el-form-item label="下行关键词">
            <el-input v-model="topicConfig.down_keyword" placeholder="down" style="width: 100px" />
          </el-form-item>
        </el-form>
      </div>

      <el-table :data="topics" style="width: 100%" stripe>
        <el-table-column label="序号" width="60" type="index" />
        <el-table-column label="名称" width="130">
          <template #default="{ row }">
            <el-input v-model="row.display_name" size="small" />
          </template>
        </el-table-column>
        <el-table-column label="方向" width="90">
          <template #default="{ row }">
            <el-select v-model="row.direction" size="small">
              <el-option label="上行" value="up" />
              <el-option label="下行" value="down" />
            </el-select>
          </template>
        </el-table-column>
        <el-table-column label="路径模板" min-width="350">
          <template #default="{ row }">
            <div class="topic-path-group">
              <span class="topic-prefix-display">{{ topicConfig.prefix }}/</span>
              <span v-if="topicConfig.show_direction" class="topic-prefix-display">{{ getDirectionKeyword(row.direction) }}/</span>
              <el-input v-model="row.path" size="small" placeholder="gateway/{gatewaySn}/..." class="topic-path-input" />
            </div>
          </template>
        </el-table-column>
        <el-table-column label="完整主题预览" min-width="400">
          <template #default="{ row }">
            <code class="topic-preview">{{ buildFullTopic(row) }}</code>
          </template>
        </el-table-column>
      </el-table>

      <div class="topic-tips">
        <p>可用变量：<code>{gatewaySn}</code> — 网关SN, <code>{deviceSn}</code> — 设备SN</p>
      </div>
    </el-card>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, watch, nextTick } from 'vue'
import request from '@/utils/request'
import { ElMessage } from 'element-plus'

const form = reactive({
  broker: '',
  protocol: 'mqtt://',
  host: '',
  port: 1883,
  username: '',
  password: '',
  client_id: '',
  keep_alive: 60,
  qos: 1,
  ssl: false,
  ssl_verify: true,
  alpn_tag: '',
  cert_type: '',
  ca_file: '',
  cert_file: '',
  key_file: '',
  version: '5.0',
  connect_timeout: 10,
  auto_reconnect: true,
  reconnect_period: 4000,
  clean_start: false,
  session_expiry: 7200,
  receive_max: '',
  max_packet_size: '',
  topic_alias_max: '',
  request_response_info: false,
  request_problem_info: false
})

const mqttConnected = ref(false)
const cacheSize = ref(0)
const advancedExpanded = ref(false)
const maxPacketSizeInitialized = ref(false)

const fetchConfig = async () => {
  try {
    const res = await request.get('/mqtt/config')
    if (res.data) {
      Object.assign(form, res.data)
      // 将 0 值转换为空，避免显示 0
      if (form.receive_max === 0) form.receive_max = ''
      if (form.max_packet_size === 0) form.max_packet_size = ''
      if (form.topic_alias_max === 0) form.topic_alias_max = ''
      // 最大数据包有值时标记为已初始化
      if (form.max_packet_size !== '' && form.max_packet_size !== undefined) {
        maxPacketSizeInitialized.value = true
      } else {
        maxPacketSizeInitialized.value = false
      }
      // 根据协议自动设置 SSL 状态
      if (form.protocol === 'mqtts://') {
        form.ssl = true
      } else {
        form.ssl = false
      }
    }
  } catch (error) {
    console.error('获取配置失败:', error)
  }
}

const fetchStatus = async () => {
  try {
    const res = await request.get('/mqtt/status')
    mqttConnected.value = res.data?.connected || false
  } catch (error) {
    mqttConnected.value = false
  }
}

const onReceiveMaxChange = (val) => {
  if (val === '') {
    form.receive_max = ''
  }
}

const onMaxPacketSizeChange = (val) => {
  if (val === '') {
    form.max_packet_size = ''
    maxPacketSizeInitialized.value = false
  } else if (!maxPacketSizeInitialized.value && val === 1) {
    form.max_packet_size = 100
    maxPacketSizeInitialized.value = true
  } else {
    maxPacketSizeInitialized.value = true
  }
}

const onTopicAliasMaxChange = (val) => {
  if (val === '') {
    form.topic_alias_max = ''
  }
}

const handleSave = async () => {
  try {
    // 将空字符串转回 0，防止 Go 端 int 字段反序列化失败
    const payload = { ...form }
    if (payload.receive_max === '') payload.receive_max = 0
    if (payload.max_packet_size === '') payload.max_packet_size = 0
    if (payload.topic_alias_max === '') payload.topic_alias_max = 0

    await request.put('/mqtt/config', payload)
    ElMessage.success('配置已保存')
  } catch (error) {
    console.error('保存配置失败:', error)
    ElMessage.error('保存配置失败')
  }
}

const handleConnect = async () => {
  try {
    // 将空字符串转回 0，防止 Go 端 int 字段反序列化失败
    const payload = { ...form }
    if (payload.receive_max === '') payload.receive_max = 0
    if (payload.max_packet_size === '') payload.max_packet_size = 0
    if (payload.topic_alias_max === '') payload.topic_alias_max = 0

    if (mqttConnected.value) {
      await request.post('/mqtt/disconnect', payload)
      ElMessage.success('已断开连接')
    } else {
      await request.post('/mqtt/connect', payload)
      ElMessage.success('连接成功')
    }
    fetchStatus()
  } catch (error) {
    ElMessage.error(mqttConnected.value ? '断开连接失败' : '连接失败')
  }
}

// 生成 Client ID
const generateClientId = () => {
  const sn = 'GWD' + Math.random().toString(36).substring(2, 10).toUpperCase()
  const timestamp = Date.now().toString().slice(-8)
  form.client_id = `${sn}_${timestamp}`
}

// SSL 开关变化时同步协议
const onSSLChange = (val) => {
  if (val) {
    form.protocol = 'mqtts://'
    // 默认选择 CA已签署服务器证书
    if (!form.cert_type) {
      form.cert_type = 'ca_signed'
    }
  } else {
    form.protocol = 'mqtt://'
  }
  window.getSelection()?.removeAllRanges()
}

// 协议变化时同步 SSL
watch(() => form.protocol, (val) => {
  if (val === 'mqtts://') {
    form.ssl = true
  } else {
    form.ssl = false
  }
  window.getSelection()?.removeAllRanges()
})

// 全新开始开关变化时同步会话过期间隔
const onCleanStartChange = (val) => {
  if (val) {
    form.session_expiry = 0
  } else {
    form.session_expiry = 7200
  }
}

// 切换高级配置显示
const toggleAdvanced = () => {
  advancedExpanded.value = !advancedExpanded.value
}

// 文件选择
const selectFile = (field) => {
  const input = document.createElement('input')
  input.type = 'file'
  input.accept = '.crt,.key,.pem,.jks,.der,.cer,.pfx'
  input.onchange = (e) => {
    const file = e.target.files[0]
    if (file) {
      form[field] = file.name
    }
  }
  input.click()
}

// ─── 主题配置 ────────────────────────────
const topics = ref([])
const topicConfig = reactive({
  prefix: '/aixot',
  up_keyword: 'up',
  down_keyword: 'down',
  show_direction: true
})

const getDirectionKeyword = (direction) => {
  return direction === 'up' ? topicConfig.up_keyword : topicConfig.down_keyword
}

const buildFullTopic = (row) => {
  let topic = topicConfig.prefix
  if (topicConfig.show_direction) {
    topic += '/' + getDirectionKeyword(row.direction)
  }
  topic += '/' + row.path
  if (row.custom_part) {
    topic += row.custom_part
  }
  return topic
}

const fetchTopics = async () => {
  try {
    const [topicsRes, configRes] = await Promise.all([
      request.get('/mqtt/topics'),
      request.get('/mqtt/topic-config')
    ])
    
    if (topicsRes.data && topicsRes.data.length > 0) {
      topics.value = topicsRes.data
    }
    
    if (configRes.data) {
      Object.assign(topicConfig, configRes.data)
    }
  } catch (error) {
    console.error('获取主题配置失败:', error)
  }
}

const handleSaveTopics = async () => {
  try {
    await Promise.all([
      request.put('/mqtt/topic-config', topicConfig),
      request.put('/mqtt/topics', topics.value.map(t => ({
        ...t,
        is_default: false
      })))
    ])
    ElMessage.success('主题配置已保存')
  } catch (error) {
    console.error('保存主题失败:', error)
    ElMessage.error('保存主题失败')
  }
}

const handleResetTopics = async () => {
  try {
    const [topicsRes, configRes] = await Promise.all([
      request.post('/mqtt/topics/reset'),
      request.post('/mqtt/topic-config/reset')
    ])
    
    if (topicsRes.data) {
      topics.value = topicsRes.data
    }
    
    if (configRes.data) {
      Object.assign(topicConfig, configRes.data)
    }
    
    ElMessage.success('已恢复默认主题')
  } catch (error) {
    console.error('恢复默认主题失败:', error)
    ElMessage.error('恢复默认主题失败')
  }
}

onMounted(() => {
  fetchConfig()
  fetchTopics()
  fetchStatus()
  setInterval(fetchStatus, 60000)
})
</script>

<style scoped>
.mqtt-config {
  padding: 20px;
}

.config-section {
  margin-bottom: 20px;
}

.section-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px;
  background-color: #f5f5f5;
  border-radius: 4px;
  cursor: pointer;
  margin-bottom: 16px;
}

.section-header h4 {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
}

.host-input-group {
  display: flex;
  gap: 8px;
}

.protocol-select {
  width: 150px;
}

.client-id-group {
  display: flex;
  gap: 8px;
  width: 100%;
}

.client-id-group .el-input {
  flex: 1;
}

.file-input-group {
  display: flex;
  gap: 8px;
  width: 110%;
}

.file-input-group .el-input {
  flex: 1;
}

.ssl-content {
  padding: 0 16px;
  user-select: none;
  -webkit-user-select: none;
  -moz-user-select: none;
  -ms-user-select: none;
}

.cert-content {
  padding: 0 16px;
}

.advanced-content {
  padding-top: 16px;
}

.action-buttons {
  margin-top: 20px;
  padding-top: 20px;
  border-top: 1px solid #eee;
}

.status-info {
  padding: 20px;
  font-size: 16px;
}

.slide-enter-active,
.slide-leave-active {
  transition: all 0.3s ease;
}

.slide-enter-from,
.slide-leave-to {
  opacity: 0;
  max-height: 0;
}

.slide-enter-to,
.slide-leave-from {
  opacity: 1;
  max-height: 1000px;
}

/* ─── 主题配置 ─── */
.topic-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.topic-global {
  margin-bottom: 16px;
  padding: 16px;
  background-color: #fafafa;
  border-radius: 8px;
}

.topic-global-form {
  flex-wrap: wrap;
  gap: 8px;
}

.topic-path-group {
  display: flex;
  align-items: center;
  gap: 4px;
}

.topic-prefix-display {
  color: #909399;
  font-size: 13px;
  white-space: nowrap;
  font-family: 'Monaco', 'Menlo', monospace;
}

.topic-path-input {
  flex: 1;
}

.topic-preview {
  font-size: 13px;
  color: #409eff;
  word-break: break-all;
  font-family: 'Monaco', 'Menlo', monospace;
}

.topic-tips {
  margin-top: 12px;
  padding: 12px 16px;
  background-color: #f5f7fa;
  border-radius: 8px;
  font-size: 13px;
  color: #909399;
}

.topic-tips code {
  color: #409eff;
  background: #ecf5ff;
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 12px;
  font-family: 'Monaco', 'Menlo', monospace;
}
</style>
