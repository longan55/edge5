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
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, watch } from 'vue'
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
    await request.put('/mqtt/config', form)
    ElMessage.success('配置已保存')
  } catch (error) {
    console.error('保存配置失败:', error)
    ElMessage.error('保存配置失败')
  }
}

const handleConnect = async () => {
  try {
    if (mqttConnected.value) {
      await request.post('/mqtt/disconnect', form)
      ElMessage.success('已断开连接')
    } else {
      await request.post('/mqtt/connect', form)
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

onMounted(() => {
  fetchConfig()
  fetchStatus()
  setInterval(fetchStatus, 5000)
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
</style>