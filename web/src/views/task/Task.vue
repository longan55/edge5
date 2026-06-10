<template>
  <div class="task-list">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>采集任务</span>
          <el-button type="primary" @click="handleAdd">新增任务</el-button>
        </div>
      </template>

      <!-- 搜索栏 -->
      <div class="search-box">
        <el-form :inline="true" :model="searchInfo" @keyup.enter="onSubmit">
          <el-form-item label="任务名称">
            <el-input v-model="searchInfo.name" placeholder="搜索条件" clearable />
          </el-form-item>
          <el-form-item>
            <el-button type="primary" @click="onSubmit">查询</el-button>
            <el-button @click="onReset">重置</el-button>
          </el-form-item>
        </el-form>
      </div>

      <!-- 表格 -->
      <div class="table-box">
        <el-table :data="tableData" style="width: 100%" @selection-change="handleSelectionChange">
          <el-table-column type="expand" width="60">
            <template #default="{ row }">
              <div class="expand-content" v-if="row.commands && row.commands.length">
                <el-table :data="row.commands" border size="small">
                  <el-table-column label="参数名" prop="name" width="120" />
                  <el-table-column label="地址" prop="address" width="120" />
                  <el-table-column label="偏移量" prop="offset" width="100" />
                  <el-table-column label="解析类型" prop="parseType" width="120" />
                </el-table>
              </div>
              <div v-else style="padding: 10px; color: #999;">暂无采集参数</div>
            </template>
          </el-table-column>
          <el-table-column type="selection" width="50" />
          <el-table-column label="ID" prop="id" width="60" />
          <el-table-column label="任务名称" prop="name" width="180" />
          <el-table-column label="关联设备" prop="deviceName" width="180" />
          <el-table-column label="上传主题" prop="upTopic" width="180" />
          <el-table-column label="读取间隔" prop="readInterval" width="100">
            <template #default="{ row }">{{ row.readInterval }}s</template>
          </el-table-column>
          <el-table-column label="状态" prop="state" width="100">
            <template #default="{ row }">
              <el-tag :type="row.state ? 'success' : 'danger'" size="small">
                {{ row.state ? '运行中' : '已停止' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="280" fixed="right">
            <template #default="{ row }">
              <el-button type="success" size="small" link @click="handleStart(row)">启动</el-button>
              <el-button type="warning" size="small" link @click="handleStop(row)">停止</el-button>
              <el-button type="primary" size="small" link @click="handleEdit(row)">编辑</el-button>
              <el-button type="danger" size="small" link @click="handleDelete(row)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
      </div>

      <!-- 分页 -->
      <div class="pagination">
        <el-pagination
          layout="total, sizes, prev, pager, next, jumper"
          :current-page="page"
          :page-size="pageSize"
          :page-sizes="[10, 20, 50]"
          :total="total"
          @current-change="handleCurrentChange"
          @size-change="handleSizeChange"
        />
      </div>
    </el-card>

    <!-- 新增/编辑任务弹窗 -->
    <el-dialog
      :title="dialogType === 'create' ? '新增任务' : '编辑任务'"
      v-model="dialogVisible"
      width="800px"
      :close-on-click-modal="false"
      @close="closeDialog"
    >
      <el-form :model="formData" label-width="120px" ref="formRef" :rules="rules">
        <el-form-item label="任务名称" prop="name">
          <el-input v-model="formData.name" placeholder="请输入任务名称" />
        </el-form-item>
        <el-form-item label="关联设备" prop="deviceId">
          <el-select v-model="formData.deviceId" placeholder="请选择设备" clearable filterable style="width: 100%" @change="handleDeviceChange">
            <el-option v-for="d in deviceList" :key="d.id" :label="d.device_name" :value="d.id" />
          </el-select>
          <div v-if="!deviceList.length" style="color: #999; font-size: 12px; margin-top: 4px;">暂无设备，请先添加设备</div>
        </el-form-item>
        <el-form-item label="上传主题" prop="upTopic">
          <el-input v-model="formData.upTopic" placeholder="可选，默认自动生成" />
        </el-form-item>
        <el-form-item label="读取间隔(秒)" prop="readInterval">
          <el-input-number v-model="formData.readInterval" :min="1" :max="3600" />
        </el-form-item>

        <!-- 采集参数列表 — 动态渲染 -->
        <el-form-item label="采集参数">
          <div class="params-section">
            <div class="params-header">
              <span class="params-title">参数列表</span>
              <el-button type="primary" size="small" @click="addCommand">添加参数</el-button>
            </div>
            <el-alert v-if="!formData.deviceId" title="请先选择关联设备" type="info" show-icon :closable="false" style="margin-bottom: 10px;" />
            <el-alert v-else-if="!readParamsSchema.length" :title="'该协议未定义采集参数类型，使用默认参数'" type="warning" show-icon :closable="false" style="margin-bottom: 10px;" />
            <el-table :data="formData.commands" border size="small" class="params-table" v-if="formData.commands.length">
              <!-- name 是通用字段 -->
              <el-table-column label="参数名" width="140">
                <template #default="{ row, $index }">
                  <el-input v-model="row.name" placeholder="参数名" size="small" />
                </template>
              </el-table-column>
              <!-- 动态渲染 schema 中定义的字段 -->
              <el-table-column v-for="field in readParamsSchema" :key="field.name" :label="field.cName || field.name" :width="field.type === 'int' ? 130 : 140">
                <template #default="{ row, $index }">
                  <!-- string 类型 -->
                  <el-input v-if="field.type === 'string'" v-model="row[field.name]" :placeholder="field.cName || field.name" size="small" />
                  <!-- int 类型 -->
                  <el-input-number v-else-if="field.type === 'int'" v-model="row[field.name]" :min="0" :max="65535" size="small" style="width: 100%" />
                  <!-- select 类型 -->
                  <el-select v-else-if="field.type === 'select'" v-model="row[field.name]" size="small" style="width: 100%">
                    <el-option v-for="c in field.choices" :key="c" :label="c" :value="c" />
                  </el-select>
                  <!-- 默认 string -->
                  <el-input v-else v-model="row[field.name]" :placeholder="field.cName" size="small" />
                </template>
              </el-table-column>
              <el-table-column label="操作" width="80">
                <template #default="{ $index }">
                  <el-button type="danger" size="small" @click="removeCommand($index)">删除</el-button>
                </template>
              </el-table-column>
            </el-table>
            <div v-if="!formData.commands.length" style="text-align: center; padding: 20px; color: #999;">
              暂无采集参数，点击"添加参数"按钮添加
            </div>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="closeDialog">取消</el-button>
        <el-button type="primary" :loading="btnLoading" @click="submitForm">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  createTask,
  updateTask,
  deleteTask,
  getTask,
  getTasks,
  startTask,
  stopTask,
  getDevices,
  getReadParamsSchema
} from '@/api/task'

// 搜索
const searchInfo = ref({ name: '' })
const page = ref(1)
const pageSize = ref(10)
const total = ref(0)
const tableData = ref([])
const multipleSelection = ref([])

// 设备列表
const deviceList = ref([])

// 采集参数的字段 schema（动态从后端获取）
const readParamsSchema = ref([])

const onSubmit = () => {
  page.value = 1
  fetchData()
}

const onReset = () => {
  searchInfo.value = { name: '' }
  page.value = 1
  fetchData()
}

const handleSelectionChange = (val) => {
  multipleSelection.value = val
}

const handleCurrentChange = (val) => {
  page.value = val
  fetchData()
}

const handleSizeChange = (val) => {
  pageSize.value = val
  fetchData()
}

const fetchData = async () => {
  try {
    const res = await getTasks({ page: page.value, pageSize: pageSize.value, ...searchInfo.value })
    if (res.code === 0) {
      const data = res.data
      const tasks = data.tasks || []
      tableData.value = tasks.map(t => ({
        ...t,
        deviceName: getDeviceName(t.deviceId)
      }))
      total.value = data.total || 0
    }
  } catch (e) {
    console.error('获取任务列表失败', e)
  }
}

const getDeviceName = (deviceId) => {
  const d = deviceList.value.find(item => item.id === deviceId)
  return d ? d.device_name : deviceId
}

const fetchDevices = async () => {
  try {
    const res = await getDevices({ page: 1, pageSize: 1000 })
    if (res.code === 0) {
      // response.Page 返回 { list: [...], total: N }
      const list = res.data?.list || res.data || []
      deviceList.value = list
    }
  } catch (e) {
    console.error('获取设备列表失败', e)
  }
}

// 表单
const dialogVisible = ref(false)
const dialogType = ref('create')
const btnLoading = ref(false)
const formRef = ref(null)
const formData = ref({
  id: 0,
  name: '',
  deviceId: '',
  upTopic: '',
  readInterval: 10,
  state: false,
  commands: []
})

const rules = {
  name: [{ required: true, message: '请输入任务名称', trigger: 'blur' }],
  deviceId: [{ required: true, message: '请选择设备', trigger: 'change' }]
}

const handleAdd = () => {
  dialogType.value = 'create'
  formData.value = {
    id: 0,
    name: '',
    deviceId: '',
    upTopic: '',
    readInterval: 10,
    state: false,
    commands: []
  }
  readParamsSchema.value = []
  dialogVisible.value = true
}

const handleEdit = async (row) => {
  dialogType.value = 'update'
  try {
    const res = await getTask(row.id)
    if (res.code === 0) {
      const task = res.data
      formData.value = {
        id: task.id,
        name: task.name,
        deviceId: task.deviceId,
        upTopic: task.upTopic,
        readInterval: task.readInterval,
        state: task.state,
        commands: task.commands || []
      }
      // 加载设备协议的采集参数 schema
      if (task.deviceId) {
        await loadReadParamsSchema(task.deviceId)
      }
      dialogVisible.value = true
    }
  } catch (e) {
    console.error('获取任务详情失败', e)
  }
}

const handleDeviceChange = async (deviceId) => {
  if (!deviceId) {
    formData.value.commands = []
    readParamsSchema.value = []
    return
  }
  // 清空之前的参数
  formData.value.commands = []
  await loadReadParamsSchema(deviceId)
  // 默认添加一行参数（用户在编辑已有任务时不会触发）
  if (dialogType.value === 'create') {
    addCommand()
  }
}

const loadReadParamsSchema = async (deviceId) => {
  try {
    const res = await getReadParamsSchema(deviceId)
    if (res.code === 0) {
      readParamsSchema.value = res.data?.readParamsSchema || []
    }
  } catch (e) {
    console.error('获取采集参数 schema 失败', e)
    readParamsSchema.value = []
  }
}

const addCommand = () => {
  const cmd = { name: '' }
  // 用 schema 填充默认值
  for (const field of readParamsSchema.value) {
    cmd[field.name] = field.default ?? (field.type === 'int' ? 0 : '')
  }
  formData.value.commands.push(cmd)
}

const removeCommand = (index) => {
  formData.value.commands.splice(index, 1)
}

const closeDialog = () => {
  dialogVisible.value = false
  readParamsSchema.value = []
  formRef.value?.resetFields()
}

const submitForm = async () => {
  formRef.value?.validate(async (valid) => {
    if (!valid) return
    btnLoading.value = true
    try {
      let res
      if (dialogType.value === 'create') {
        res = await createTask(formData.value)
      } else {
        res = await updateTask(formData.value.id, formData.value)
      }
      if (res.code === 0) {
        ElMessage.success(dialogType.value === 'create' ? '创建成功' : '更新成功')
        closeDialog()
        fetchData()
      } else {
        ElMessage.error(res.message || '操作失败')
      }
    } catch (e) {
      ElMessage.error('操作失败')
    } finally {
      btnLoading.value = false
    }
  })
}

const handleStart = async (row) => {
  try {
    const res = await startTask(row.id)
    if (res.code === 0) {
      ElMessage.success('任务已启动')
      fetchData()
    } else {
      ElMessage.error(res.message || '启动失败')
    }
  } catch (e) {
    ElMessage.error('启动失败')
  }
}

const handleStop = async (row) => {
  try {
    const res = await stopTask(row.id)
    if (res.code === 0) {
      ElMessage.success('任务已停止')
      fetchData()
    } else {
      ElMessage.error(res.message || '停止失败')
    }
  } catch (e) {
    ElMessage.error('停止失败')
  }
}

const handleDelete = (row) => {
  ElMessageBox.confirm('确定删除该任务吗？', '提示', {
    type: 'warning'
  }).then(async () => {
    try {
      const res = await deleteTask(row.id)
      if (res.code === 0) {
        ElMessage.success('删除成功')
        fetchData()
      }
    } catch (e) {
      ElMessage.error('删除失败')
    }
  }).catch(() => {})
}

// 初始化
onMounted(() => {
  fetchData()
  fetchDevices()
})
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.search-box {
  margin-bottom: 20px;
}

.table-box {
  margin-bottom: 20px;
}

.pagination {
  display: flex;
  justify-content: flex-end;
}

.expand-content {
  padding: 10px 20px;
  background-color: #f8f9fa;
}

.params-section {
  border: 1px solid #e4e7ed;
  border-radius: 4px;
  padding: 15px;
  background-color: #fafafa;
  width: 100%;
}

.params-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 15px;
}

.params-title {
  font-weight: bold;
  font-size: 14px;
}

.params-table {
  width: 100%;
}
</style>
