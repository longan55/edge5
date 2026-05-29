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
            <el-option label="PLC" value="plc" />
            <el-option label="CNC" value="cnc" />
          </el-select>
        </el-form-item>
        <el-form-item label="品牌">
          <el-select v-model="filters.brand" placeholder="全部" clearable>
            <el-option label="三菱" value="mitsubishi" />
            <el-option label="西门子" value="siemens" />
            <el-option label="Fanuc" value="fanuc" />
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

    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="760px" destroy-on-close>
      <el-form :model="deviceForm" :rules="formRules" ref="formRef" label-width="100px">
        <el-form-item label="设备SN" prop="device_sn">
          <el-input v-model="deviceForm.device_sn" :disabled="!!deviceForm.id" />
        </el-form-item>

        <el-form-item label="设备名称" prop="device_name">
          <el-input v-model="deviceForm.device_name" />
        </el-form-item>

        <el-form-item label="设备类型" prop="device_type">
          <el-select v-model="deviceForm.device_type">
            <el-option label="PLC" value="plc" />
            <el-option label="CNC" value="cnc" />
          </el-select>
        </el-form-item>

        <el-form-item label="品牌" prop="brand">
          <el-select v-model="deviceForm.brand">
            <el-option label="三菱" value="mitsubishi" />
            <el-option label="西门子" value="siemens" />
            <el-option label="Fanuc" value="fanuc" />
          </el-select>
        </el-form-item>

        <el-form-item label="协议" prop="protocol">
          <el-radio-group v-model="deviceForm.protocol">
            <el-radio label="tcp">TCP</el-radio>
            <el-radio label="serial">Serial</el-radio>
          </el-radio-group>
        </el-form-item>

        <el-divider />

        <div v-if="templateLoading" style="padding: 12px 0; color: #999">
          加载设备模板中...
        </div>

        <div v-else-if="deviceForm.device_type === 'plc'">
          <el-card shadow="never" style="margin-bottom: 12px">
            <template #header>
              <div>运行时配置（Runtime）</div>
            </template>

            <template v-if="deviceForm.protocol === 'tcp'">
              <el-form-item label="IP地址">
                <el-input v-model="deviceForm.config.runtime.ip" />
              </el-form-item>
              <el-form-item label="端口">
                <el-input-number v-model="deviceForm.config.runtime.port" :min="1" :max="65535" />
              </el-form-item>

              <el-form-item label="插件gRPC Host">
                <el-input v-model="deviceForm.config.runtime.extra.host" placeholder="127.0.0.1" />
              </el-form-item>
              <el-form-item label="插件gRPC Port">
                <el-input-number v-model="deviceForm.config.runtime.extra.port" :min="1" :max="65535" />
              </el-form-item>
            </template>

            <template v-else>
              <el-form-item label="串口">
                <el-input v-model="deviceForm.config.runtime.serial_port" placeholder="/dev/ttyS0" />
              </el-form-item>
              <el-form-item label="波特率">
                <el-select v-model="deviceForm.config.runtime.baud_rate">
                  <el-option :value="9600" label="9600" />
                  <el-option :value="19200" label="19200" />
                  <el-option :value="38400" label="38400" />
                  <el-option :value="115200" label="115200" />
                </el-select>
              </el-form-item>

              <el-form-item label="插件gRPC Host">
                <el-input v-model="deviceForm.config.runtime.extra.host" placeholder="127.0.0.1" />
              </el-form-item>
              <el-form-item label="插件gRPC Port">
                <el-input-number v-model="deviceForm.config.runtime.extra.port" :min="1" :max="65535" />
              </el-form-item>
            </template>
          </el-card>

          <el-card shadow="never">
            <template #header>
              <div>采集配置（Collection）</div>
            </template>

            <el-form-item label="采集间隔(ms)">
              <el-input-number v-model="deviceForm.config.collection.intervalMs" :min="100" :step="100" />
            </el-form-item>

            <div style="margin-top: 12px; margin-bottom: 8px; display:flex; align-items:center; justify-content:space-between">
              <div style="font-weight: 600">点位列表</div>
              <el-button size="small" @click="addPlcPoint">添加点位</el-button>
            </div>

            <el-card
              v-for="(p, idx) in deviceForm.config.collection.points"
              :key="idx"
              shadow="never"
              style="margin-bottom: 10px; border: 1px solid #eee"
            >
              <div style="display:flex; gap: 12px; flex-wrap: wrap; align-items:center">
                <el-form-item label="key" style="flex: 1; min-width: 180px">
                  <el-input v-model="p.key" />
                </el-form-item>
                <el-form-item label="中文名" style="flex: 1; min-width: 180px">
                  <el-input v-model="p.zhName" />
                </el-form-item>
                <el-form-item label="address" style="flex: 1; min-width: 180px">
                  <el-input v-model="p.address" />
                </el-form-item>
                <el-form-item label="type" style="width: 190px">
                  <el-select v-model="p.type">
                    <el-option label="int16" value="int16" />
                    <el-option label="uint16" value="uint16" />
                    <el-option label="int32" value="int32" />
                    <el-option label="uint32" value="uint32" />
                    <el-option label="float" value="float" />
                  </el-select>
                </el-form-item>
                <el-form-item label="offset" style="width: 160px">
                  <el-input-number v-model="p.offset" :step="1" />
                </el-form-item>
                <el-form-item label="scale" style="width: 160px">
                  <el-input-number v-model="p.scale" :step="0.1" />
                </el-form-item>
                <el-form-item label="unit" style="width: 160px">
                  <el-input v-model="p.unit" />
                </el-form-item>

                <el-button type="danger" size="small" @click="removePlcPoint(idx)" :disabled="deviceForm.config.collection.points.length <= 1">
                  删除
                </el-button>
              </div>
            </el-card>
          </el-card>
        </div>

        <div v-else-if="deviceForm.device_type === 'cnc'">
          <el-card shadow="never" style="margin-bottom: 12px">
            <template #header>
              <div>运行时配置（Runtime）</div>
            </template>

            <el-form-item label="扩展参数(JSON)">
              <el-input
                v-model="cncExtraText"
                type="textarea"
                :rows="4"
                placeholder="{ }"
                @blur="applyCncExtra"
              />
            </el-form-item>
          </el-card>

          <el-card shadow="never">
            <template #header>
              <div>采集配置（Collection）</div>
            </template>

            <el-form-item label="采集间隔(ms)">
              <el-input-number v-model="deviceForm.config.collection.intervalMs" :min="100" :step="100" />
            </el-form-item>

            <div style="margin-top: 12px; margin-bottom: 8px; display:flex; align-items:center; justify-content:space-between">
              <div style="font-weight: 600">启用字段列表</div>
              <el-button size="small" @click="addCncField">添加字段</el-button>
            </div>

            <el-card
              v-for="(f, idx) in deviceForm.config.collection.fields"
              :key="idx"
              shadow="never"
              style="margin-bottom: 10px; border: 1px solid #eee"
            >
              <div style="display:flex; gap: 12px; flex-wrap: wrap; align-items:center">
                <el-form-item label="key" style="width: 220px">
                  <el-select v-model="f.key">
                    <el-option label="spindleSpeed" value="spindleSpeed" />
                    <el-option label="feedRate" value="feedRate" />
                    <el-option label="alarmCode" value="alarmCode" />
                  </el-select>
                </el-form-item>
                <el-form-item label="中文名" style="flex: 1; min-width: 220px">
                  <el-input v-model="f.zhName" />
                </el-form-item>
                <el-button type="danger" size="small" @click="removeCncField(idx)" :disabled="deviceForm.config.collection.fields.length <= 1">
                  删除
                </el-button>
              </div>
            </el-card>
          </el-card>
        </div>

        <template #footer>
          <el-button @click="dialogVisible = false">取消</el-button>
          <el-button type="primary" @click="handleSubmit">确定</el-button>
        </template>
      </el-form>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted, watch } from 'vue'
import request from '@/utils/request'
import { ElMessage, ElMessageBox } from 'element-plus'

const loading = ref(false)
const deviceList = ref([])
const dialogVisible = ref(false)
const formRef = ref(null)

const templateLoading = ref(false)
const templateData = reactive({
  schema: null,
  defaultConfig: null
})

const cncExtraText = ref('{}')

const filters = reactive({
  deviceType: '',
  brand: ''
})

const pagination = reactive({
  page: 1,
  pageSize: 10,
  total: 0
})

const deviceForm = reactive({
  id: null,
  device_sn: '',
  device_name: '',
  device_type: 'plc',
  brand: 'mitsubishi',
  protocol: 'tcp',
  config: {
    runtime: {},
    collection: {}
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

const mergeWithTemplate = (existingConfig) => {
  const base = deepClone(templateData.defaultConfig || {})
  if (!existingConfig || typeof existingConfig !== 'object') {
    deviceForm.config = base
    return
  }

  // new config format: { runtime: {...}, collection: {...} }
  if (existingConfig.runtime || existingConfig.collection) {
    base.runtime = { ...(base.runtime || {}), ...(existingConfig.runtime || {}) }
    base.collection = { ...(base.collection || {}), ...(existingConfig.collection || {}) }
    deviceForm.config = base
    return
  }

  // old config format: directly runtime fields (ip/port or serial_port/baud_rate)
  base.runtime = { ...(base.runtime || {}), ...existingConfig }
  deviceForm.config = base
}

const fetchTemplate = async () => {
  templateLoading.value = true
  try {
    const res = await request.get('/device/template', {
      params: {
        device_type: deviceForm.device_type,
        protocol: deviceForm.protocol
      }
    })
    templateData.schema = res.data?.schema || null
    templateData.defaultConfig = res.data?.default_config || {}

    mergeWithTemplate(deviceForm.config)
    // apply cnc extra text for ui
    if (deviceForm.device_type === 'cnc') {
      cncExtraText.value = JSON.stringify(deviceForm.config?.runtime?.extra || {}, null, 2)
    }
  } catch (e) {
    console.error('加载模板失败:', e)
    ElMessage.error('加载设备模板失败')
  } finally {
    templateLoading.value = false
  }
}

const addPlcPoint = () => {
  deviceForm.config.collection.points.push({
    key: '',
    zhName: '',
    address: '',
    type: 'int16',
    offset: 0,
    scale: 1,
    unit: ''
  })
}

const removePlcPoint = (idx) => {
  if (deviceForm.config.collection.points.length <= 1) return
  deviceForm.config.collection.points.splice(idx, 1)
}

const addCncField = () => {
  deviceForm.config.collection.fields.push({
    key: 'spindleSpeed',
    zhName: ''
  })
}

const removeCncField = (idx) => {
  if (deviceForm.config.collection.fields.length <= 1) return
  deviceForm.config.collection.fields.splice(idx, 1)
}

const applyCncExtra = () => {
  if (deviceForm.device_type !== 'cnc') return
  try {
    const v = JSON.parse(cncExtraText.value || '{}')
    deviceForm.config.runtime.extra = v
  } catch {
    ElMessage.error('扩展参数 JSON 格式错误')
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

const initFormForAdd = () => {
  deviceForm.id = null
  deviceForm.device_sn = ''
  deviceForm.device_name = ''
  deviceForm.device_type = 'plc'
  deviceForm.brand = 'mitsubishi'
  deviceForm.protocol = 'tcp'
  deviceForm.config = {
    runtime: {
      extra: {
        host: '',
        port: 50051
      }
    },
    collection: {}
  }
}

const handleAdd = () => {
  initFormForAdd()
  dialogVisible.value = true
  // wait dialog open then fetch
  fetchTemplate()
}

const handleEdit = (row) => {
  deviceForm.id = row.id
  deviceForm.device_sn = row.device_sn
  deviceForm.device_name = row.device_name
  deviceForm.device_type = row.device_type
  deviceForm.brand = row.brand
  deviceForm.protocol = row.protocol

  try {
    const parsed = JSON.parse(row.config || '{}')
    deviceForm.config = parsed && typeof parsed === 'object' ? parsed : { runtime: {}, collection: {} }
  } catch {
    deviceForm.config = { runtime: {}, collection: {} }
  }

  // ensure runtime.extra exists for v-model binding
  if (!deviceForm.config.runtime || typeof deviceForm.config.runtime !== 'object') {
    deviceForm.config.runtime = {}
  }
  if (!deviceForm.config.runtime.extra || typeof deviceForm.config.runtime.extra !== 'object') {
    deviceForm.config.runtime.extra = { host: '', port: 50051 }
  }

  dialogVisible.value = true
  fetchTemplate()
}

watch(
  () => [deviceForm.device_type, deviceForm.protocol, dialogVisible.value],
  async ([type, proto, open]) => {
    if (!open) return
    // keep current config by mergeWithTemplate() using template defaults
    await fetchTemplate()
    if (type === 'cnc') {
      cncExtraText.value = JSON.stringify(deviceForm.config?.runtime?.extra || {}, null, 2)
    }
  }
)

const handleSubmit = async () => {
  await formRef.value.validate()

  // ensure required shape
  if (!deviceForm.config || typeof deviceForm.config !== 'object') {
    ElMessage.error('配置格式错误')
    return
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

const handleStart = async (row) => {
  const action = row.status === 1 ? 'stop' : 'start'
  try {
    await request.post(`/device/${row.id}/${action}`)
    ElMessage.success(row.status === 1 ? '设备已停用' : '设备已启用')
    fetchDevices()
  } catch (error) {
    console.error('操作失败:', error)
  }
}

const handleDelete = async (row) => {
  await ElMessageBox.confirm('确定要删除该设备吗？', '提示', { type: 'warning' })
  try {
    await request.delete(`/device/${row.id}`)
    ElMessage.success('删除成功')
    fetchDevices()
  } catch (error) {
    console.error('删除失败:', error)
  }
}

const handleRowClick = (row) => {
  // placeholder
}

onMounted(() => {
  fetchDevices()
})
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
</style>
