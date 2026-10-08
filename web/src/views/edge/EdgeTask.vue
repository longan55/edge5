<template>
  <div class="edge-task-list">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>边缘计算</span>
          <el-button type="primary" @click="handleAdd">新增计算任务</el-button>
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
        <el-table :data="tableData" style="width: 100%">
          <el-table-column type="expand" width="60">
            <template #default="{ row }">
              <div class="expand-content" v-if="row.rules && row.rules.length">
                <el-table :data="ruleRows(row)" border size="small">
                  <el-table-column label="输出字段" prop="name" width="140" />
                  <el-table-column label="计算方式" prop="funcLabel" width="160" />
                  <el-table-column label="引用点位">
                    <template #default="{ row: rr }">
                      <el-tag v-for="v in rr.vars" :key="v.var" size="small" style="margin-right: 6px;">
                        {{ v.var }}<span v-if="v.threshold != null"> ≥{{ v.threshold }}</span>
                      </el-tag>
                    </template>
                  </el-table-column>
                  <el-table-column label="参数" width="260">
                    <template #default="{ row: rr }">{{ formatParams(rr) }}</template>
                  </el-table-column>
                </el-table>
              </div>
              <div v-else style="padding: 10px; color: #999;">暂无计算规则</div>
            </template>
          </el-table-column>
          <el-table-column label="ID" prop="id" width="60" />
          <el-table-column label="任务名称" prop="name" width="180" />
          <el-table-column label="关联设备" width="180">
            <template #default="{ row }">{{ deviceName(row.deviceId) }}</template>
          </el-table-column>
          <el-table-column label="结果上报主题" prop="upTopic" min-width="220" show-overflow-tooltip />
          <el-table-column label="采样" width="80">
            <template #default="{ row }">{{ row.sampleInterval || row.readInterval || 1 }}s</template>
          </el-table-column>
          <el-table-column label="上报" width="80">
            <template #default="{ row }">{{ row.reportInterval || 10 }}s</template>
          </el-table-column>
          <el-table-column label="状态" width="100">
            <template #default="{ row }">
              <el-tag :type="stateTagType(row.status)" size="small">{{ stateText(row.status) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="300" fixed="right">
            <template #default="{ row }">
              <el-button type="success" size="small" link @click="handleStart(row)">启动</el-button>
              <el-button type="warning" size="small" link @click="handleStop(row)">停止</el-button>
              <el-button type="primary" size="small" link @click="handleEdit(row)">编辑</el-button>
              <el-button type="info" size="small" link @click="handleViewData(row)">数据</el-button>
              <el-button type="danger" size="small" link @click="handleDelete(row)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
      </div>

      <div class="pagination">
        <el-pagination
          layout="total, sizes, prev, pager, next, jumper"
          :current-page="page"
          :page-size="pageSize"
          :page-sizes="[10, 20, 50]"
          :total="total"
          @current-change="onPageChange"
          @size-change="onSizeChange"
        />
      </div>
    </el-card>

    <!-- 新增/编辑对话框 -->
    <el-dialog
      :title="dialogType === 'create' ? '新增边缘计算任务' : '编辑边缘计算任务'"
      v-model="dialogVisible"
      width="900px"
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
        <el-form-item label="结果上报主题">
          <el-input v-model="formData.upTopic" placeholder="可选，默认 /aixot/up/{网关SN}/{设备SN}/edge/data" />
        </el-form-item>
        <el-form-item label="采样间隔(秒)">
          <el-input-number v-model="formData.sampleInterval" :min="1" :max="60" />
          <span style="margin-left: 8px; color: #999; font-size: 12px;">读设备频率，计数依赖边沿检测建议 1-2s</span>
        </el-form-item>
        <el-form-item label="上报间隔(秒)">
          <el-input-number v-model="formData.reportInterval" :min="1" :max="3600" />
          <span style="margin-left: 8px; color: #999; font-size: 12px;">MQTT 上报频率，避免刷屏</span>
        </el-form-item>

        <!-- 采集点位池：顶层声明所有要采的点位（变量名/地址/解析/表达式），规则按变量名引用 -->
        <el-form-item label="采集点位">
          <div class="rules-section">
            <div class="rules-header">
              <span class="rules-title">点位池（统一声明，规则按变量名引用，相同地址只采一次）</span>
              <el-button type="primary" size="small" @click="addInput">添加点位</el-button>
            </div>
            <el-alert v-if="!formData.deviceId" title="请先选择关联设备后再添加点位" type="info" show-icon :closable="false" style="margin-bottom: 10px;" />
            <el-table :data="formData.inputs" border size="small">
              <el-table-column label="变量名" width="120">
                <template #default="{ row }">
                  <el-input v-model="row.var" size="small" placeholder="如 a / 电流" />
                </template>
              </el-table-column>
              <el-table-column label="地址" min-width="160">
                <template #default="{ row }">
                  <el-input v-model="row.address" size="small" placeholder="如 0x3A / 40059" />
                </template>
              </el-table-column>
              <el-table-column label="解析类型" width="150">
                <template #default="{ row }">
                  <el-select v-model="row.parseType" size="small" filterable allow-create default-first-option style="width: 100%">
                    <el-option v-for="t in parseTypeChoices" :key="t" :label="t" :value="t" />
                  </el-select>
                </template>
              </el-table-column>
              <el-table-column width="190">
                <template #header>
                  <span>表达式(x)</span>
                  <el-tooltip placement="top" content="采样后计算，x 为读到的原始值，支持 + - * / 与括号，如 x*0.1、(x-4)*2.5+20；留空为原值">
                    <el-icon class="tip-icon"><QuestionFilled /></el-icon>
                  </el-tooltip>
                </template>
                <template #default="{ row }">
                  <el-input v-model="row.expr" size="small" placeholder="如 x*0.1，留空原值" />
                </template>
              </el-table-column>
              <el-table-column label="操作" width="70">
                <template #default="{ $index }">
                  <el-button type="danger" size="small" link @click="removeInput($index)">删除</el-button>
                </template>
              </el-table-column>
            </el-table>
            <div v-if="!formData.inputs.length" style="text-align: center; padding: 16px; color: #999;">
              暂无点位，点击"添加点位"
            </div>
          </div>
        </el-form-item>

        <el-form-item label="计算规则">
          <div class="rules-section">
            <div class="rules-header">
              <span class="rules-title">规则列表（每条规则产生一个计算后的上报字段，通过变量名引用点位池）</span>
              <el-button type="primary" size="small" @click="addRule">添加规则</el-button>
            </div>

            <el-card v-for="(rule, ri) in formData.rules" :key="ri" shadow="never" class="rule-card">
              <template #header>
                <div class="rule-card-header">
                  <span>规则 {{ ri + 1 }}</span>
                  <el-button type="danger" size="small" link @click="removeRule(ri)">删除规则</el-button>
                </div>
              </template>

              <el-row :gutter="12">
                <el-col :span="9">
                  <div class="field-label">输出字段名</div>
                  <el-input v-model="rule.name" placeholder="如：冲压次数" size="small" />
                </el-col>
                <el-col :span="9">
                  <div class="field-label">计算方式</div>
                  <el-select v-model="rule.funcType" size="small" style="width: 100%" @change="changeFunc(rule)">
                    <el-option v-for="f in funcMetas" :key="f.type" :label="f.cName" :value="f.type" />
                  </el-select>
                </el-col>
              </el-row>

              <div class="func-desc" v-if="funcMeta(rule.funcType)?.desc">{{ funcMeta(rule.funcType).desc }}</div>

              <!-- 算子参数（动态渲染） -->
              <div class="params-grid" v-if="funcParams(rule.funcType).length">
                <div v-for="p in funcParams(rule.funcType)" :key="p.name" class="param-item" v-show="showParam(rule, p)">
                  <div class="field-label">
                    {{ p.cName }}
                    <el-tooltip v-if="p.desc" :content="p.desc" placement="top">
                      <el-icon class="tip-icon"><QuestionFilled /></el-icon>
                    </el-tooltip>
                  </div>
                  <el-input-number v-if="p.type === 'number'" v-model="rule.params[p.name]" size="small" :controls="false" style="width: 100%" />
                  <el-select v-else-if="p.type === 'select'" v-model="rule.params[p.name]" size="small" style="width: 100%">
                    <el-option v-for="o in p.options" :key="String(o.value)" :label="o.label" :value="o.value" />
                  </el-select>
                  <el-switch v-else-if="p.type === 'bool'" v-model="rule.params[p.name]" size="small" />
                  <el-input v-else v-model="rule.params[p.name]" size="small" :placeholder="p.desc || p.cName" />
                </div>
              </div>

              <!-- counter_analog 在线分析按钮（任务需启动后才能分析，因为窗口在算子内存中） -->
              <div v-if="isAnalog(rule.funcType)" style="margin-top: 8px;">
                <el-button type="primary" size="small" plain :loading="analyzingRule === rule.name" @click="analyzeRulePeak(rule)">
                  分析峰谷（基于最近样本）
                </el-button>
                <span v-if="rule._analyzeHint" style="margin-left: 10px; color: #67c23a; font-size: 12px;">{{ rule._analyzeHint }}</span>
              </div>

              <!-- 引用变量（从点位池下拉选择，可选带本点阈值）。引用多个点位时自动变为多点位组合计数 -->
              <div class="field-label" style="margin-top: 10px;">
                引用变量（从点位池选择）
                <span v-if="isCounter(rule.funcType) && isMultiVar(rule)" style="color: #e6a23c;">
                  — 已为多点位计数，请在上方选择组合条件（与/或/满足N个）
                </span>
              </div>
              <el-table :data="rule.vars" border size="small">
                <el-table-column label="变量" min-width="160">
                  <template #default="{ row }">
                    <el-select v-model="row.var" size="small" filterable allow-create default-first-option placeholder="选择点位池变量" style="width: 100%">
                      <el-option v-for="i in formData.inputs" :key="i.var" :label="`${i.var} (${i.address})`" :value="i.var" />
                    </el-select>
                  </template>
                </el-table-column>
                <el-table-column v-if="showThresholdCol(rule)" :label="thresholdColLabel(rule)" width="190">
                  <template #default="{ row }">
                    <el-input-number v-model="row.threshold" size="small" :controls="false" :value-on-clear="null" :placeholder="thresholdColPlaceholder(rule)" style="width: 100%" />
                  </template>
                </el-table-column>
                <el-table-column label="操作" width="70">
                  <template #default="{ $index }">
                    <el-button type="danger" size="small" link @click="removeVar(rule, $index)">删除</el-button>
                  </template>
                </el-table-column>
              </el-table>
              <el-button size="small" style="margin-top: 8px;" @click="addVar(rule)">添加引用（多个点位自动变为组合计数）</el-button>
            </el-card>

            <div v-if="!formData.rules.length" style="text-align: center; padding: 24px; color: #999;">
              暂无规则，点击"添加规则"
            </div>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="closeDialog">取消</el-button>
        <el-button type="primary" :loading="btnLoading" @click="submitForm">确定</el-button>
      </template>
    </el-dialog>

    <!-- 数据查看 -->
    <el-dialog :title="'计算结果 - ' + currentTaskName" v-model="dataDialogVisible" width="900px" :close-on-click-modal="false">
      <el-table :data="dataList" style="width: 100%">
        <el-table-column label="时间" width="200">
          <template #default="{ row }">{{ formatTime(row.timestamp) }}</template>
        </el-table-column>
        <el-table-column label="计算结果" prop="data">
          <template #default="{ row }">
            <pre style="white-space: pre-wrap; word-break: break-all; margin: 0; font-family: inherit;">{{ formatData(row.data) }}</pre>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="row.upState ? 'success' : 'danger'" size="small">{{ row.upState ? '已上报' : '缓存中' }}</el-tag>
          </template>
        </el-table-column>
      </el-table>
      <div v-if="!dataList.length" style="text-align: center; padding: 40px; color: #999;">暂无数据</div>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { QuestionFilled } from '@element-plus/icons-vue'
import {
  createEdgeTask, updateEdgeTask, deleteEdgeTask, getEdgeTask, getEdgeTasks,
  startEdgeTask, stopEdgeTask, getEdgeTaskData, getEdgeFunctions,
  getDevices, getReadParamsSchema, analyzeEdgeRule
} from '@/api/edge'

const FALLBACK_TYPES = ['bool', 'byte', 'int16', 'uint16', 'int32', 'uint32', 'int64', 'uint64', 'float32', 'float64', 'string', 'short', 'ushort', 'int', 'uint', 'long', 'ulong', 'float', 'double']

const searchInfo = ref({ name: '' })
const page = ref(1)
const pageSize = ref(10)
const total = ref(0)
const tableData = ref([])
const deviceList = ref([])
const funcMetas = ref([])
const readParamsSchema = ref([])

const parseTypeChoices = computed(() => {
  const f = (readParamsSchema.value || []).find(p => p.name === 'parseType' || p.name === 'parse_type')
  if (f && Array.isArray(f.choices) && f.choices.length) return f.choices
  return FALLBACK_TYPES
})

const funcMeta = (type) => funcMetas.value.find(f => f.type === type)
const funcParams = (type) => funcMeta(type)?.params || []
const isCounter = (type) => type === 'counter_digital' || type === 'counter_analog'
const isAnalog = (type) => type === 'counter_analog'
const isMultiVar = (rule) => (rule.vars?.length || 0) > 1

// 历史算子类型 → 当前类型（与后端 edge.NormalizeFuncType 保持一致）
const normalizeFuncType = (t) => {
  if (t === 'counter_pulse' || t === 'counter_multi') return 'counter_digital'
  if (t === 'counter_ac') return 'counter_analog'
  return t
}

// 参数条件显示：
//  - 组合条件 / 统一阈值：仅引用多个点位时出现（“满足个数N”还要求 logic=n_of_m）
//  - 手动峰谷：仅模拟峰值计数且关闭自动学习时出现
//  - 学习参数：仅模拟峰值计数且开启自动学习时出现
const showParam = (rule, p) => {
  const multi = isMultiVar(rule)
  switch (p.name) {
    case 'logic':
      return multi
    case 'requireCount':
      return multi && rule.params.logic === 'n_of_m'
    case 'threshold':
      return multi && rule.funcType === 'counter_digital'
    case 'peakMin':
    case 'valleyMax':
      return rule.funcType === 'counter_analog' && !rule.params.autoLearn
    case 'learnWindow':
    case 'warmupSamples':
    case 'learnInterval':
    case 'learnPercentile':
      return rule.funcType === 'counter_analog' && !!rule.params.autoLearn
    default:
      return true
  }
}

// 引用变量表中的“本点阈值”列：
//  - 数字信号计数：总是显示。单点时即峰值阈值；多点时可覆盖统一阈值
//  - 模拟峰值计数：仅单点显示（覆盖峰值最低值）；多点位共用全局峰谷
const showThresholdCol = (rule) => {
  if (rule.funcType === 'counter_digital') return true
  if (rule.funcType === 'counter_analog') return !isMultiVar(rule)
  return false
}
const thresholdColLabel = (rule) => {
  if (rule.funcType === 'counter_analog') return '本点峰值(可空)'
  return isMultiVar(rule) ? '本点阈值(可空)' : '峰值阈值'
}
const thresholdColPlaceholder = (rule) => {
  if (rule.funcType === 'counter_analog') return '用峰值最低值'
  return isMultiVar(rule) ? '用统一阈值' : '请输入阈值'
}

const deviceName = (id) => {
  const d = deviceList.value.find(x => x.id === id)
  return d ? d.device_name : id
}

const funcLabel = (type) => funcMeta(type)?.cName || type

function ruleRows(row) {
  return (row.rules || []).map(r => {
    const nr = { ...r, funcType: normalizeFuncType(r.funcType) }
    return { ...nr, funcLabel: funcLabel(nr.funcType) }
  })
}

function formatParams(rule) {
  const ps = funcParams(rule.funcType)
  return ps
    .filter(p => showParam(rule, p))
    .map(p => `${p.cName}=${rule.params?.[p.name] ?? ''}`)
    .join('，')
}

const fetchData = async () => {
  try {
    const res = await getEdgeTasks({ page: page.value, pageSize: pageSize.value, ...searchInfo.value })
    if (res.code === 0) {
      tableData.value = res.data?.tasks || []
      total.value = res.data?.total || 0
    }
  } catch (e) { console.error('获取边缘计算任务失败', e) }
}

const fetchDevices = async () => {
  try {
    const res = await getDevices({ page: 1, pageSize: 1000 })
    if (res.code === 0) {
      deviceList.value = res.data?.list || res.data || []
    }
  } catch (e) { console.error('获取设备列表失败', e) }
}

const fetchFunctions = async () => {
  try {
    const res = await getEdgeFunctions()
    if (res.code === 0) funcMetas.value = res.data || []
  } catch (e) { console.error('获取算子列表失败', e) }
}

const loadSchema = async (deviceId) => {
  try {
    const res = await getReadParamsSchema(deviceId)
    readParamsSchema.value = res.code === 0 ? (res.data?.readParamsSchema || []) : []
  } catch (e) { readParamsSchema.value = [] }
}

const onSubmit = () => { page.value = 1; fetchData() }
const onReset = () => { searchInfo.value = { name: '' }; page.value = 1; fetchData() }
const onPageChange = (v) => { page.value = v; fetchData() }
const onSizeChange = (v) => { pageSize.value = v; fetchData() }

// ---------------- 表单 ----------------
const dialogVisible = ref(false)
const dialogType = ref('create')
const btnLoading = ref(false)
const formRef = ref(null)
const formData = ref(emptyForm())

function emptyForm() {
  return { id: 0, name: '', deviceId: '', upTopic: '', sampleInterval: 1, reportInterval: 10, state: false, inputs: [], rules: [] }
}

const rules = {
  name: [{ required: true, message: '请输入任务名称', trigger: 'blur' }],
  deviceId: [{ required: true, message: '请选择设备', trigger: 'change' }]
}

function defaultParams(meta) {
  const obj = {}
  ;(meta?.params || []).forEach(p => {
    if (p.default !== undefined && p.default !== null && p.default !== '') obj[p.name] = p.default
    else obj[p.name] = p.type === 'number' ? 0 : ''
  })
  return obj
}

// 点位池：自动按字母顺序生成变量名 a/b/c...
function nextVarName() {
  const used = new Set(formData.value.inputs.map(i => i.var))
  for (let i = 0; i < 26; i++) {
    const v = String.fromCharCode(97 + i)
    if (!used.has(v)) return v
  }
  return 'v' + (formData.value.inputs.length + 1)
}

function makeInput() {
  return {
    var: nextVarName(),
    address: '',
    parseType: parseTypeChoices.value[0] || 'uint16',
    expr: ''
  }
}

// 规则内引用一个变量，可选带本点阈值
function makeVarRef() {
  return { var: '', threshold: null }
}

function makeRule(funcType) {
  const type = funcType || funcMetas.value.find(f => f.type === 'counter_digital')?.type || funcMetas.value[0]?.type
  const meta = funcMeta(type)
  const r = { name: '', funcType: type, params: defaultParams(meta), vars: [] }
  // 默认预填一个引用（如果点位池非空）
  if (formData.value.inputs.length > 0) {
    r.vars.push(makeVarRef())
  }
  return r
}

const addRule = () => {
  if (!funcMetas.value.length) { ElMessage.warning('算子未加载'); return }
  if (!formData.value.inputs.length) { ElMessage.warning('请先在点位池添加点位'); return }
  formData.value.rules.push(makeRule())
}
const removeRule = (i) => formData.value.rules.splice(i, 1)

// 点位池的增删
const addInput = () => formData.value.inputs.push(makeInput())
const removeInput = (i) => formData.value.inputs.splice(i, 1)

// 规则内引用的增删
const addVar = (rule) => rule.vars.push(makeVarRef())
const removeVar = (rule, i) => rule.vars.splice(i, 1)

const changeFunc = (rule) => {
  rule.params = defaultParams(funcMeta(rule.funcType))
}

// 在线分析 counter_analog 峰谷值（任务必须已启动，因为样本在算子内存）
const analyzingRule = ref('')
const analyzeRulePeak = async (rule) => {
  if (!formData.value.id) {
    ElMessage.warning('请先保存任务并启动后再分析')
    return
  }
  if (!rule.name) {
    ElMessage.warning('请先填写规则输出字段名')
    return
  }
  analyzingRule.value = rule.name
  try {
    const res = await analyzeEdgeRule(formData.value.id, rule.name)
    if (res.code === 0) {
      const info = res.data
      if (!info || !info.samples) {
        rule._analyzeHint = '样本不足，请启动任务采集若干样本后再分析'
        ElMessage.warning(rule._analyzeHint)
        return
      }
      // 把建议值填入表单，让用户预览后决定是否保存
      rule.params.peakMin = Number(info.suggestedPeakMin?.toFixed(3))
      rule.params.valleyMax = Number(info.suggestedValleyMax?.toFixed(3))
      const parts = [
        `样本 ${info.samples} 个`,
        `建议峰 ${rule.params.peakMin}`,
        `建议谷 ${rule.params.valleyMax}`,
      ]
      if (info.warmupDone) {
        parts.push(`已学: 峰 ${Number(info.learnedPeakMin?.toFixed(3))} / 谷 ${Number(info.learnedValleyMax?.toFixed(3))}`)
      }
      rule._analyzeHint = parts.join('，')
      ElMessage.success('已填入建议峰谷值，确认后请保存')
    } else {
      ElMessage.error(res.msg || '分析失败')
    }
  } catch (e) {
    ElMessage.error(e.message || '分析失败')
  } finally {
    analyzingRule.value = ''
  }
}

const handleAdd = async () => {
  await fetchDevices()
  dialogType.value = 'create'
  formData.value = emptyForm()
  readParamsSchema.value = []
  dialogVisible.value = true
}

const handleDeviceChange = async (deviceId) => {
  if (!deviceId) { formData.value.inputs = []; formData.value.rules = []; readParamsSchema.value = []; return }
  await loadSchema(deviceId)
  // 地址属于具体设备协议，切换设备时清空点位池与规则
  formData.value.inputs = []
  formData.value.rules = []
}

const handleEdit = async (row) => {
  await fetchDevices()
  dialogType.value = 'update'
  try {
    const res = await getEdgeTask(row.id)
    if (res.code === 0) {
      const t = res.data
      formData.value = {
        id: t.id, name: t.name, deviceId: t.deviceId, upTopic: t.upTopic,
        sampleInterval: t.sampleInterval || t.readInterval || 1,
        reportInterval: t.reportInterval || 10,
        state: t.state,
        inputs: (t.inputs || []).map(i => {
          // 优先用新表达式；旧倍率数据（scale≠1）回填为 x*scale
          let exprStr = i.expr || ''
          if (!exprStr && i.scale && Number(i.scale) !== 1) {
            exprStr = 'x*' + String(i.scale)
          }
          return { var: i.var, address: i.address, parseType: i.parseType, expr: exprStr }
        }),
        rules: (t.rules || []).map(r => ({
          name: r.name,
          funcType: normalizeFuncType(r.funcType),
          params: r.params || {},
          vars: (r.vars || []).map(v => ({ var: v.var, threshold: v.threshold ?? null }))
        }))
      }
      if (t.deviceId) await loadSchema(t.deviceId)
      dialogVisible.value = true
    }
  } catch (e) { console.error('获取任务详情失败', e) }
}

const closeDialog = () => {
  dialogVisible.value = false
  readParamsSchema.value = []
  formRef.value?.resetFields()
}

function validateRules() {
  if (!formData.value.rules.length) return '请至少添加一条计算规则'
  const names = new Set()
  const inputVars = new Set(formData.value.inputs.map(i => i.var))
  for (let i = 0; i < formData.value.rules.length; i++) {
    const r = formData.value.rules[i]
    if (!r.name) return `第 ${i + 1} 条规则缺少输出字段名`
    if (names.has(r.name)) return `输出字段名重复：${r.name}`
    names.add(r.name)
    if (!r.funcType) return `规则 ${r.name} 未选择计算方式`
    if (!r.vars || !r.vars.length) return `规则 ${r.name} 至少需要引用一个变量`
    const vars = new Set()
    for (const vr of r.vars) {
      if (!vr.var) return `规则 ${r.name} 存在未选择变量的引用`
      if (!inputVars.has(vr.var)) return `规则 ${r.name} 引用了未声明的变量：${vr.var}`
      if (vars.has(vr.var)) return `规则 ${r.name} 重复引用了同一变量：${vr.var}`
      vars.add(vr.var)
    }
    if (r.funcType === 'arithmetic' && !r.params?.expression) return `规则 ${r.name} 未填写表达式`
  }
  return ''
}

const submitForm = () => {
  formRef.value?.validate(async (valid) => {
    if (!valid) return
    const err = validateRules()
    if (err) { ElMessage.error(err); return }
    btnLoading.value = true
    try {
      const res = dialogType.value === 'create'
        ? await createEdgeTask(formData.value)
        : await updateEdgeTask(formData.value.id, formData.value)
      if (res.code === 0) {
        ElMessage.success(dialogType.value === 'create' ? '创建成功' : '更新成功')
        closeDialog()
        fetchData()
      } else {
        ElMessage.error(res.message || '操作失败')
      }
    } catch (e) {
      ElMessage.error(e.response?.data?.message || '操作失败')
    } finally {
      btnLoading.value = false
    }
  })
}

const handleStart = async (row) => {
  try {
    const res = await startEdgeTask(row.id)
    if (res.code === 0) { ElMessage.success('任务已启动'); fetchData() }
    else ElMessage.error(res.message || '启动失败')
  } catch (e) { ElMessage.error(e.response?.data?.message || '启动失败') }
}
const handleStop = async (row) => {
  try {
    const res = await stopEdgeTask(row.id)
    if (res.code === 0) { ElMessage.success('任务已停止'); fetchData() }
    else ElMessage.error(res.message || '停止失败')
  } catch (e) { ElMessage.error(e.response?.data?.message || '停止失败') }
}
const handleDelete = (row) => {
  ElMessageBox.confirm('确定删除该边缘计算任务吗？', '提示', { type: 'warning' })
    .then(async () => {
      const res = await deleteEdgeTask(row.id)
      if (res.code === 0) { ElMessage.success('删除成功'); fetchData() }
    }).catch(() => {})
}

// ---------------- 数据查看 ----------------
const dataDialogVisible = ref(false)
const currentTaskName = ref('')
const currentTaskId = ref(null)
const dataList = ref([])
let dataTimer = null
let listTimer = null

const fetchTaskData = async () => {
  if (!currentTaskId.value) return
  const res = await getEdgeTaskData(currentTaskId.value)
  if (res.code === 0) dataList.value = res.data || []
}
const handleViewData = async (row) => {
  currentTaskName.value = row.name
  currentTaskId.value = row.id
  dataList.value = []
  await fetchTaskData()
  if (dataTimer) clearInterval(dataTimer)
  dataTimer = setInterval(fetchTaskData, 2000)
  dataDialogVisible.value = true
}
watch(dataDialogVisible, (v) => {
  if (!v && dataTimer) { clearInterval(dataTimer); dataTimer = null; currentTaskId.value = null }
})

const formatTime = (ts) => ts ? new Date(ts).toLocaleString('zh-CN') : ''
const formatData = (s) => {
  if (!s) return ''
  try { return JSON.stringify(JSON.parse(s)) } catch (e) { return s }
}

const stateTagType = (s) => s === 'running' ? 'success' : s === 'disconnected' ? 'warning' : 'danger'
const stateText = (s) => s === 'running' ? '运行中' : s === 'disconnected' ? '设备未连接' : '已停止'

onMounted(() => {
  fetchData()
  fetchDevices()
  fetchFunctions()
  listTimer = setInterval(fetchData, 5000)
})
onUnmounted(() => {
  if (listTimer) clearInterval(listTimer)
  if (dataTimer) clearInterval(dataTimer)
})
</script>

<style scoped>
.card-header { display: flex; justify-content: space-between; align-items: center; }
.search-box { margin-bottom: 20px; }
.table-box { margin-bottom: 20px; }
.pagination { display: flex; justify-content: flex-end; }
.expand-content { padding: 10px 20px; background-color: #f8f9fa; }
.rules-section { border: 1px solid #e4e7ed; border-radius: 4px; padding: 15px; background-color: #fafafa; width: 100%; }
.rules-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px; }
.rules-title { font-weight: bold; font-size: 14px; }
.rule-card { margin-bottom: 12px; }
.rule-card-header { display: flex; justify-content: space-between; align-items: center; }
.field-label { font-size: 12px; color: #606266; margin-bottom: 4px; }
.func-desc { font-size: 12px; color: #909399; margin: 8px 0; }
.params-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 10px; margin-top: 10px; }
.param-item { min-width: 0; }
.tip-icon { vertical-align: -2px; color: #909399; cursor: help; }
</style>
