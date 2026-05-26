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

    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="600px">
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

        <template v-if="deviceForm.protocol === 'tcp'">
          <el-form-item label="IP地址">
            <el-input v-model="deviceForm.config.ip" />
          </el-form-item>
          <el-form-item label="端口">
            <el-input-number v-model="deviceForm.config.port" :min="1" :max="65535" />
          </el-form-item>
        </template>

        <template v-else>
          <el-form-item label="串口">
            <el-input v-model="deviceForm.config.serial_port" placeholder="/dev/ttyS0" />
          </el-form-item>
          <el-form-item label="波特率">
            <el-select v-model="deviceForm.config.baud_rate">
              <el-option label="9600" :value="9600" />
              <el-option label="19200" :value="19200" />
              <el-option label="38400" :value="38400" />
              <el-option label="115200" :value="115200" />
            </el-select>
          </el-form-item>
        </template>
      </el-form>

      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="handleSubmit">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted } from 'vue'
import request from '@/utils/request'
import { ElMessage, ElMessageBox } from 'element-plus'

const loading = ref(false)
const deviceList = ref([])
const dialogVisible = ref(false)
const formRef = ref(null)

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
    ip: '',
    port: 6000,
    serial_port: '/dev/ttyS0',
    baud_rate: 9600
  }
})

const dialogTitle = computed(() => deviceForm.id ? '编辑设备' : '新增设备')

const formRules = {
  device_sn: [{ required: true, message: '请输入设备SN', trigger: 'blur' }],
  device_name: [{ required: true, message: '请输入设备名称', trigger: 'blur' }],
  device_type: [{ required: true, message: '请选择设备类型', trigger: 'change' }],
  brand: [{ required: true, message: '请选择品牌', trigger: 'change' }],
  protocol: [{ required: true, message: '请选择协议', trigger: 'change' }]
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

const handleAdd = () => {
  deviceForm.id = null
  deviceForm.device_sn = ''
  deviceForm.device_name = ''
  deviceForm.device_type = 'plc'
  deviceForm.brand = 'mitsubishi'
  deviceForm.protocol = 'tcp'
  deviceForm.config = {
    ip: '',
    port: 6000,
    serial_port: '/dev/ttyS0',
    baud_rate: 9600
  }
  dialogVisible.value = true
}

const handleEdit = (row) => {
  deviceForm.id = row.id
  deviceForm.device_sn = row.device_sn
  deviceForm.device_name = row.device_name
  deviceForm.device_type = row.device_type
  deviceForm.brand = row.brand
  deviceForm.protocol = row.protocol
  try {
    deviceForm.config = JSON.parse(row.config || '{}')
  } catch {
    deviceForm.config = {}
  }
  dialogVisible.value = true
}

const handleSubmit = async () => {
  await formRef.value.validate()

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
  await ElMessageBox.confirm('确定要删除该设备吗？', '提示', {
    type: 'warning'
  })

  try {
    await request.delete(`/device/${row.id}`)
    ElMessage.success('删除成功')
    fetchDevices()
  } catch (error) {
    console.error('删除失败:', error)
  }
}

const handleRowClick = (row) => {
  // 可以跳转到设备详情页
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
