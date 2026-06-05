<template>
  <div class="device-list">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>设备列表</span>
          <el-button type="primary" @click="handleAdd">新增设备</el-button>
        </div>
      </template>

      <el-form inline @submit.prevent>
        <el-form-item label="设备类型">
          <el-select v-model="filters.deviceType" placeholder="全部" clearable>
            <el-option
              v-for="dt in deviceOptions.deviceTypes"
              :key="dt.value"
              :label="dt.label"
              :value="dt.value"
            />
          </el-select>
        </el-form-item>

        <el-form-item label="品牌">
          <el-select v-model="filters.brand" placeholder="全部" clearable>
            <el-option
              v-for="b in allBrandOptions"
              :key="b.value"
              :label="b.label"
              :value="b.value"
            />
          </el-select>
        </el-form-item>

        <el-form-item>
          <el-button @click="handleFilter">筛选</el-button>
          <el-button @click="handleReset">重置</el-button>
        </el-form-item>
      </el-form>

      <el-table :data="deviceList" v-loading="loading" @row-click="handleRowClick">
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column prop="device_sn" label="设备SN" />
        <el-table-column prop="device_name" label="设备名称" />
        <el-table-column prop="device_type" label="类型" width="100">
          <template #default="{ row }">
            <el-tag>{{ row.device_type?.toUpperCase() }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="brand" label="品牌" width="100" />
        <el-table-column prop="protocol" label="协议" width="100" />
        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="row.status === 1 ? 'success' : 'info'">
              {{ row.status === 1 ? '启用' : '禁用' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="在线状态" width="120">
          <template #default="{ row }">
            <el-tag :type="row.online ? 'success' : 'danger'" size="small">
              {{ row.online ? '在线' : '离线' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="last_heartbeat" label="最后心跳" width="180" />
        <el-table-column label="操作" width="280" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click.stop="handleStart(row)">
              {{ row.status === 1 ? '停用' : '启用' }}
            </el-button>
            <el-button link type="primary" @click.stop="handleEdit(row)">编辑</el-button>
            <el-button link type="danger" @click.stop="handleDelete(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-pagination
        v-model:current-page="pagination.page"
        v-model:page-size="pagination.pageSize"
        :total="pagination.total"
        @current-change="fetchDevices"
        @size-change="fetchDevices"
        layout="total, sizes, prev, pager, next"
        style="margin-top: 20px; justify-content: flex-end"
      />
    </el-card>

    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="860px" destroy-on-close>
      <el-form :model="deviceForm" :rules="formRules" ref="formRef" label-width="120px">
        <el-form-item label="设备SN" prop="device_sn">
          <el-input v-model="deviceForm.device_sn" :disabled="!!deviceForm.id" />
        </el-form-item>

        <el-form-item label="设备名称" prop="device_name">
          <el-input v-model="deviceForm.device_name" />
        </el-form-item>

        <el-divider />

        <el-form-item label="设备类型" prop="device_type">
          <el-select
            v-model="deviceForm.device_type"
            placeholder="请选择"
            @change="handleDeviceTypeChange"
          >
            <el-option v-for="dt in deviceOptions.deviceTypes" :key="dt.value" :label="dt.label" :value="dt.value" />
          </el-select>
        </el-form-item>

        <el-form-item label="品牌" prop="brand">
          <el-select
            v-model="deviceForm.brand"
            placeholder="请选择"
            :disabled="brandOptions.length === 0"
            @change="handleBrandChange"
          >
            <el-option v-for="b in brandOptions" :key="b.value" :label="b.label" :value="b.value" />
          </el-select>
        </el-form-item>

        <el-form-item label="协议" prop="protocol">
          <el-select
            v-model="deviceForm.protocol"
            placeholder="请选择"
            :disabled="protocolOptions.length === 0"
            @change="handleProtocolChange"
          >
            <el-option v-for="p in protocolOptions" :key="p.value" :label="p.label" :value="p.value" />
          </el-select>
        </el-form-item>

        <el-form-item v-if="modelRelated" label="型号" prop="model">
          <el-select v-model="deviceForm.model" placeholder="请选择" :disabled="modelOptions.length === 0">
            <el-option v-for="m in modelOptions" :key="m" :label="m" :value="m" />
          </el-select>
        </el-form-item>

        <el-divider />

        <el-card shadow="never" style="margin-bottom: 12px">
          <template #header>
            <div>连接参数（设备侧）</div>
          </template>

          <el-alert v-if="!deviceForm.protocol" title="请选择协议后显示连接参数" type="info" show-icon />
          <template v-else>
            <el-row :gutter="12">
              <el-col
                v-for="opt in protocolConnParams"
                :key="opt.name"
                :span="12"
                style="margin-bottom: 10px"
              >
                <el-form-item :label="opt.cName" :required="opt.required">
                  <el-select
                    v-if="opt.choices && opt.choices.length > 0"
                    v-model="deviceForm.config[opt.name]"
                    style="width: 100%"
                  >
                    <el-option
                      v-for="c in opt.choices"
                      :key="String(c)"
                      :label="String(c)"
                      :value="c"
                    />
                  </el-select>

                  <el-input
                    v-else-if="opt.type === 'string'"
                    v-model="deviceForm.config[opt.name]"
                    :placeholder="typeof opt.default !== 'undefined' ? String(opt.default) : ''"
                  />
                  <el-input-number
                    v-else-if="opt.type === 'int'"
                    v-model="deviceForm.config[opt.name]"
                    :min="1"
                    :max="2147483647"
                    :step="1"
                  />
                  <el-input-number
                    v-else-if="opt.type === 'float'"
                    v-model="deviceForm.config[opt.name]"
                    :step="0.1"
                  />
                  <el-input v-else v-model="deviceForm.config[opt.name]" />
                </el-form-item>
              </el-col>
            </el-row>
          </template>
        </el-card>
      </el-form>

      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="handleSubmit">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import request from '@/utils/request'
import { ElMessage, ElMessageBox } from 'element-plus'

const loading = ref(false)
const deviceList = ref([])
const dialogVisible = ref(false)
const formRef = ref(null)

const deviceOptions = reactive({
  deviceTypes: [],
  protocolOptions: {}
})

const optionsLoading = ref(false)

const filters = reactive({
  deviceType: '',
  brand: ''
})

const pagination = reactive({
  page: 1,
  pageSize: 10,
  total: 0
})

const DEFAULT_PLUGIN_HOST = '127.0.0.1'
const DEFAULT_PLUGIN_PORT = 50051

const deviceForm = reactive({
  id: null,
  device_sn: '',
  device_name: '',
  device_type: '',
  brand: '',
  protocol: '',
  model: '',
  config: {
    pluginHost: DEFAULT_PLUGIN_HOST,
    pluginPort: DEFAULT_PLUGIN_PORT
  }
})

const dialogTitle = computed(() => (deviceForm.id ? '编辑设备' : '新增设备'))

const formRules = {
  device_sn: [{ required: true, message: '请输入设备SN', trigger: 'blur' }],
  device_name: [{ required: true, message: '请输入设备名称', trigger: 'blur' }],
  device_type: [{ required: true, message: '请选择设备类型', trigger: 'change' }],
  brand: [{ required: true, message: '请选择品牌', trigger: 'change' }],
  protocol: [{ required: true, message: '请选择协议', trigger: 'change' }]
}

const deepClone = obj => JSON.parse(JSON.stringify(obj || {}))

const selectedDeviceType = computed(() => {
  if (!deviceForm.device_type) return null
  return deviceOptions.deviceTypes.find(d => d.value === deviceForm.device_type) || null
})

const brandOptions = computed(() => selectedDeviceType.value?.brands || [])

const protocolOptions = computed(() => {
  if (!deviceForm.brand) return []
  const brand = brandOptions.value.find(b => b.value === deviceForm.brand)
  return brand?.protocols || []
})

const modelRelated = computed(() => {
  const p = protocolOptions.value.find(p => p.value === deviceForm.protocol)
  return !!p?.modelRelated
})

const modelOptions = computed(() => {
  const p = protocolOptions.value.find(p => p.value === deviceForm.protocol)
  return p?.models || []
})

const protocolConnParams = computed(() => {
  if (!deviceForm.protocol) return []
  const group = deviceOptions.protocolOptions?.[deviceForm.protocol]
  return group?.options || []
})

const allBrandOptions = computed(() => {
  const map = new Map()
  for (const dt of deviceOptions.deviceTypes) {
    for (const b of dt.brands || []) {
      if (!map.has(b.value)) map.set(b.value, b)
    }
  }
  return Array.from(map.values())
})

const normalizeDeviceType = v => {
  if (!v) return ''
  const s = String(v).toLowerCase()
  if (s === 'plc') return 'PLC'
  if (s === 'cnc') return 'CNC'
  if (v === 'PLC' || v === 'CNC') return v
  return v
}

const normalizeBrand = v => {
  if (!v) return ''
  const s = String(v).toLowerCase()
  if (s === 'mitsubishi') return 'Mitsubishi'
  if (s === 'siemens') return 'Siemens'
  if (s === 'fanuc') return 'Fanuc'
  return v
}

const normalizeProtocol = (v, deviceType) => {
  if (!v) return ''
  const s = String(v)
  const lower = s.toLowerCase()
  if (lower === 'tcp') {
    if (deviceType === 'PLC') return 'MC-3E'
    if (deviceType === 'CNC') return 'Melsec-CNC'
  }
  if (lower === 'serial') {
    if (deviceType === 'PLC') return 'FX-Serial'
  }
  return s
}

const ensureConfigShape = () => {
  if (!deviceForm.config || typeof deviceForm.config !== 'object') {
    deviceForm.config = { pluginHost: DEFAULT_PLUGIN_HOST, pluginPort: DEFAULT_PLUGIN_PORT }
  }
  if (!deviceForm.config.pluginHost) deviceForm.config.pluginHost = DEFAULT_PLUGIN_HOST
  if (!deviceForm.config.pluginPort) deviceForm.config.pluginPort = DEFAULT_PLUGIN_PORT
}

const clearProtocolRuntimeParams = () => {
  if (!deviceForm.config || typeof deviceForm.config !== 'object') return
  const allowed = new Set(protocolConnParams.value.map(p => p.name))
  allowed.add('pluginHost')
  allowed.add('pluginPort')
  allowed.add('model')

  for (const k of Object.keys(deviceForm.config)) {
    if (!allowed.has(k)) delete deviceForm.config[k]
  }
}

const applyProtocolDefaults = () => {
  ensureConfigShape()
  const params = protocolConnParams.value

  const allowed = new Set(params.map(p => p.name))
  allowed.add('pluginHost')
  allowed.add('pluginPort')
  allowed.add('model')

  for (const k of Object.keys(deviceForm.config)) {
    if (!allowed.has(k)) delete deviceForm.config[k]
  }

  for (const opt of params) {
    const curr = deviceForm.config[opt.name]
    const hasValue = curr !== undefined && curr !== null && curr !== ''
    if (!hasValue && opt.default !== undefined) {
      deviceForm.config[opt.name] = deepClone(opt.default)
    } else if (!hasValue) {
      deviceForm.config[opt.name] = (opt.type === 'int' || opt.type === 'float') ? 0 : ''
    }
  }

  if (modelRelated.value) {
    if (!deviceForm.config.model && deviceForm.model) deviceForm.config.model = deviceForm.model
    if (!deviceForm.config.model && modelOptions.value.length > 0) {
      deviceForm.config.model = modelOptions.value[0]
      deviceForm.model = modelOptions.value[0]
    }
  } else {
    delete deviceForm.config.model
  }
}

const migrateExistingConfigForProtocol = () => {
  ensureConfigShape()
  const cfg = deviceForm.config

  // flatten old {runtime:{extra:{host,port}, ...}}
  const oldRuntime = cfg.runtime
  if (oldRuntime && typeof oldRuntime === 'object') {
    for (const k of Object.keys(oldRuntime)) {
      if (k === 'extra') continue
      if (cfg[k] === undefined) cfg[k] = oldRuntime[k]
    }
    if (oldRuntime.extra && typeof oldRuntime.extra === 'object') {
      if (!cfg.pluginHost) cfg.pluginHost = oldRuntime.extra.host || DEFAULT_PLUGIN_HOST
      if (!cfg.pluginPort) cfg.pluginPort = oldRuntime.extra.port || DEFAULT_PLUGIN_PORT
    }
    delete cfg.runtime
  }

  // migrate old serial field names
  if (deviceForm.device_type === 'PLC' && deviceForm.protocol === 'FX-Serial') {
    if (cfg.serial_port !== undefined && cfg.serialPort === undefined) cfg.serialPort = cfg.serial_port
    if (cfg.baud_rate !== undefined && cfg.baudRate === undefined) cfg.baudRate = cfg.baud_rate
  }

  applyProtocolDefaults()
}

const handleDeviceTypeChange = () => {
  deviceForm.brand = ''
  deviceForm.protocol = ''
  deviceForm.model = ''
  ensureConfigShape()
  clearProtocolRuntimeParams()
}

const handleBrandChange = () => {
  deviceForm.protocol = ''
  deviceForm.model = ''
  ensureConfigShape()
  clearProtocolRuntimeParams()
}

const handleProtocolChange = () => {
  deviceForm.model = ''
  ensureConfigShape()
  applyProtocolDefaults()
  if (modelRelated.value && modelOptions.value.length > 0) {
    deviceForm.model = modelOptions.value[0]
    deviceForm.config.model = deviceForm.model
  }
}

const fetchDeviceOptions = async () => {
  optionsLoading.value = true
  try {
    const res = await request.get('/device/options')
    deviceOptions.deviceTypes = res.data?.deviceTypes || []
    deviceOptions.protocolOptions = res.data?.protocolOptions || {}
  } catch (e) {
    console.error('获取设备选项失败:', e)
    ElMessage.error('获取设备选项失败')
  } finally {
    optionsLoading.value = false
  }
}

const initFormForAdd = () => {
  deviceForm.id = null
  deviceForm.device_sn = ''
  deviceForm.device_name = ''
  deviceForm.device_type = ''
  deviceForm.brand = ''
  deviceForm.protocol = ''
  deviceForm.model = ''
  deviceForm.config = {
    pluginHost: DEFAULT_PLUGIN_HOST,
    pluginPort: DEFAULT_PLUGIN_PORT
  }
}

const handleAdd = async () => {
  initFormForAdd()
  if (!deviceOptions.deviceTypes.length) {
    await fetchDeviceOptions()
  }
  dialogVisible.value = true
}

const handleEdit = async row => {
  deviceForm.id = row.id
  deviceForm.device_sn = row.device_sn
  deviceForm.device_name = row.device_name
  deviceForm.device_type = normalizeDeviceType(row.device_type)
  deviceForm.brand = normalizeBrand(row.brand)
  deviceForm.protocol = normalizeProtocol(row.protocol, deviceForm.device_type)
  deviceForm.model = ''

  try {
    const parsed = JSON.parse(row.config || '{}')
    deviceForm.config = (parsed && typeof parsed === 'object') ? parsed : { pluginHost: DEFAULT_PLUGIN_HOST, pluginPort: DEFAULT_PLUGIN_PORT }
  } catch {
    deviceForm.config = { pluginHost: DEFAULT_PLUGIN_HOST, pluginPort: DEFAULT_PLUGIN_PORT }
  }

  ensureConfigShape()

  if (!deviceOptions.deviceTypes.length) {
    await fetchDeviceOptions()
  }

  // flatten & migrate old format
  migrateExistingConfigForProtocol()

  if (modelRelated.value && modelOptions.value.length > 0) {
    deviceForm.model = deviceForm.config.model || modelOptions.value[0]
    deviceForm.config.model = deviceForm.model
  }

  dialogVisible.value = true
}

const handleSubmit = async () => {
  await formRef.value.validate()
  ensureConfigShape()

  if (modelRelated.value) {
    deviceForm.config.model = deviceForm.model || deviceForm.config.model
  }

  const data = {
    device_sn: deviceForm.device_sn,
    device_name: deviceForm.device_name,
    device_type: deviceForm.device_type,
    brand: deviceForm.brand,
    protocol: deviceForm.protocol,
    config: JSON.stringify(deviceForm.config)
  }

  try {
    if (deviceForm.id) {
      await request.put(`/device/${deviceForm.id}`, data)
      ElMessage.success('更新成功')
    } else {
      await request.post('/device', data)
      ElMessage.success('创建成功')
    }
    dialogVisible.value = false
    fetchDevices()
  } catch (error) {
    console.error('保存失败:', error)
  }
}

const fetchDevices = async () => {
  loading.value = true
  try {
    const res = await request.get('/device/list', {
      params: {
        page: pagination.page,
        page_size: pagination.pageSize,
        device_type: filters.deviceType,
        brand: filters.brand
      }
    })
    deviceList.value = res.data?.list || []
    pagination.total = res.data?.total || 0
  } catch (error) {
    console.error('获取设备列表失败:', error)
  } finally {
    loading.value = false
  }
}

const handleFilter = () => {
  pagination.page = 1
  fetchDevices()
}

const handleReset = () => {
  filters.deviceType = ''
  filters.brand = ''
  handleFilter()
}

const handleRowClick = () => {}

const handleStart = async row => {
  const action = row.status === 1 ? 'stop' : 'start'
  try {
    await request.post(`/device/${row.id}/${action}`)
    ElMessage.success(row.status === 1 ? '设备已停用' : '设备已启用')
    fetchDevices()
  } catch (error) {
    console.error('操作失败:', error)
  }
}

const handleDelete = async row => {
  await ElMessageBox.confirm('确定要删除该设备吗？', '提示', { type: 'warning' })
  try {
    await request.delete(`/device/${row.id}`)
    ElMessage.success('删除成功')
    fetchDevices()
  } catch (error) {
    console.error('删除失败:', error)
  }
}

onMounted(() => {
  fetchDevices()
  fetchDeviceOptions()
})
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
</style>
