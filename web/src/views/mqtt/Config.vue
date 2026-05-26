<template>
  <div class="mqtt-config">
    <el-card>
      <template #header>
        <span>MQTT 配置</span>
      </template>

      <el-form :model="form" label-width="120px">
        <el-form-item label="Broker 地址">
          <el-input v-model="form.broker" placeholder="mqtt://127.0.0.1:1883" />
        </el-form-item>

        <el-form-item label="端口">
          <el-input-number v-model="form.port" :min="1" :max="65535" />
        </el-form-item>

        <el-form-item label="用户名">
          <el-input v-model="form.username" />
        </el-form-item>

        <el-form-item label="密码">
          <el-input v-model="form.password" type="password" show-password />
        </el-form-item>

        <el-form-item label="客户端 ID">
          <el-input v-model="form.client_id" />
        </el-form-item>

        <el-form-item label="保活时间(秒)">
          <el-input-number v-model="form.keep_alive" :min="10" :max="300" />
        </el-form-item>

        <el-form-item label="QoS">
          <el-radio-group v-model="form.qos">
            <el-radio :label="0">0 - 最多一次</el-radio>
            <el-radio :label="1">1 - 至少一次</el-radio>
            <el-radio :label="2">2 - 恰好一次</el-radio>
          </el-radio-group>
        </el-form-item>

        <el-form-item label="网关序列号">
          <el-input v-model="form.gateway_sn" />
        </el-form-item>

        <el-form-item>
          <el-button type="primary" @click="handleSave">保存配置</el-button>
          <el-button @click="handleTest">测试连接</el-button>
          <el-button :type="mqttConnected ? 'danger' : 'success'" @click="handleConnect">
            {{ mqttConnected ? '断开连接' : '连接' }}
          </el-button>
        </el-form-item>
      </el-form>

      <el-divider />

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
import { ref, reactive, onMounted } from 'vue'
import request from '@/utils/request'
import { ElMessage } from 'element-plus'

const form = reactive({
  broker: 'tcp://127.0.0.1:1883',
  port: 1883,
  username: '',
  password: '',
  client_id: 'edge5-gateway',
  keep_alive: 60,
  qos: 1,
  gateway_sn: 'GWD001'
})

const mqttConnected = ref(false)
const cacheSize = ref(0)

const fetchConfig = async () => {
  try {
    const res = await request.get('/mqtt/config')
    if (res.data) {
      Object.assign(form, res.data)
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

const handleSave = async () => {
  try {
    await request.put('/mqtt/config', form)
    ElMessage.success('配置已保存')
  } catch (error) {
    console.error('保存配置失败:', error)
  }
}

const handleTest = async () => {
  try {
    await request.post('/mqtt/test', form)
    ElMessage.success('连接测试成功')
  } catch (error) {
    ElMessage.error('连接测试失败')
  }
}

const handleConnect = async () => {
  try {
    if (mqttConnected.value) {
      await request.post('/mqtt/disconnect')
      ElMessage.success('已断开连接')
    } else {
      await request.post('/mqtt/connect')
      ElMessage.success('连接成功')
    }
    fetchStatus()
  } catch (error) {
    ElMessage.error(mqttConnected.value ? '断开连接失败' : '连接失败')
  }
}

onMounted(() => {
  fetchConfig()
  fetchStatus()
  setInterval(fetchStatus, 5000)
})
</script>

<style scoped>
.status-info {
  padding: 20px;
  font-size: 16px;
}
</style>
