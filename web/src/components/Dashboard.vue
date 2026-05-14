<template>
  <div class="dashboard-wrapper">
    <div class="refresh-controls">
      <span class="refresh-label">Refresh Interval:</span>
      <n-select 
        v-model:value="refreshInterval" 
        :options="refreshOptions" 
        size="small"
        style="width: 120px"
        @update:value="handleIntervalChange"
      />
    </div>
    <n-grid :cols="3" :x-gap="20" class="metrics-container">
      <n-gi v-for="item in sysMetrics" :key="item.label">
        <div class="metric-card-v2"> 
          <div class="metric-title">{{ item.shortLabel }}</div>
          <div class="progress-box">
            <n-progress 
              type="circle" 
              :percentage="Math.round(item.val || 0)" 
              :color="item.color" 
              :rail-color="'#1e293b'"
              :stroke-width="8"
              class="custom-progress"
            />
          </div>
          <n-button text size="tiny" :color="item.color" class="metric-btn" @click="item.action">
            <span class="icon">📊</span> {{ item.actionLabel }}
          </n-button>
        </div>
      </n-gi>
    </n-grid>

    <div class="section-container">
      <div class="section-title">
        <span class="header-icon">⭐</span>
        <span class="header-text">Top Core Service Area</span>
      </div>
      
      <n-grid :cols="3" :x-gap="20" v-if="apps.length > 0">
        <n-gi v-for="app in apps.slice(0, 3)" :key="app.id">
          <div class="service-card-item">
            <div class="card-header-top">
              <div class="service-info-box">
                <div class="app-icon">{{ getAppIcon(app.type) }}</div>
                <div class="service-names">
                  <h4>{{ app.name }}</h4>
                  <span class="sub-label">Occupying services</span>
                </div>
              </div>
              <n-tag :color="getTypeTagColor(app.type)" border-style="none" size="small" round>
                {{ getAppTypeLabel(app.type) }}
              </n-tag>
            </div>
            <div class="status-bar-dark">
              <span class="dot" :class="{ 'running': app.status === 'running' }"></span>
              <span class="status-text">{{ app.status || 'stopped' }}</span>
            </div>
            <div class="card-actions">
              <n-button 
                class="btn-start" 
                :class="{ 'btn-stop': app.status === 'running' }"
                @click="handleAction(app.id, app.status === 'running' ? 'stop' : 'start')"
              >
                {{ app.status === 'running' ? 'Stop' : 'Start' }}
              </n-button>
              <n-button class="btn-logs" @click="openLogModal(app)">Logs</n-button>
            </div>
          </div>
        </n-gi>
      </n-grid>

      <div v-else-if="isInitialLoading" class="loading-container">
        <n-spin size="large" />
      </div>

      <div v-else-if="!isInitialLoading && apps.length === 0" class="empty-data-container">
        <img src="../assets/noTopApp.svg" class="empty-svg" alt="No Data" />
        <div class="empty-text">No core services are pinned. Pin applications from the App Center.</div>
        <n-button text type="primary" class="go-link" @click="$emit('switch-tab', 'app-center')">
          Go to App Center
        </n-button>
      </div>
    </div>

    <div class="bottom-section">
      <n-card :bordered="false" class="table-card-v2">
        <div class="section-title">
          <span class="header-icon">☕</span>
          <span class="header-text">List of recently active apps</span>
        </div>

        <n-table :bordered="false" class="tatai-design-table" v-if="filteredApps.length > 0">
          <thead>
            <tr>
              <th style="width: 40%">Name</th>
              <th style="width: 30%; text-align: center">Type</th>
              <th style="width: 30%; text-align: right">Status</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="app in filteredApps" :key="app.id" class="table-row-styled">
              <td class="name-cell">
                <span class="type-icon-wrapper">{{ getAppIcon(app.type) }}</span>
                <span class="app-name-text">{{ app.name || app.jar_path }}</span>
              </td>
              <td style="text-align: center">
                <n-tag :color="getTypeTagColor(app.type)" border-style="none" size="small" round>
                  {{ getAppTypeLabel(app.type) }}
                </n-tag>
              </td>
              <td style="text-align: right">
                <div class="status-indicator-inline">
                  <span class="status-dot" :class="{ 'is-running': app.status === 'running' }"></span>
                  <span class="status-text-val">{{ app.status === 'running' ? 'Running' : 'Idle' }}</span>
                </div>
              </td>
            </tr>
          </tbody>
        </n-table>
        <div v-else-if="isInitialLoading" class="loading-container">
          <n-spin size="large" />
        </div>
        <div v-else-if="!isInitialLoading && apps.length === 0" class="empty-data-container">
          <img src="../assets/noData.svg" class="empty-svg" alt="No Data" />
          <div class="empty-text">No active applications found.</div>
        </div>
      </n-card>
    </div>

    <n-modal v-model:show="showAddModal">
      <n-card style="width: 600px" title="Add New App" :bordered="false" size="huge" role="dialog" aria-modal="true">
        <n-form :model="addForm">
          <n-form-item label="Command">
            <n-input v-model:value="addForm.command" placeholder="Enter start command" />
          </n-form-item>
        </n-form>
        <template #action>
          <n-button @click="showAddModal = false">Cancel</n-button>
          <n-button type="primary" @click="confirmAddApp">Confirm</n-button>
        </template>
      </n-card>
    </n-modal>

    <!-- 日志弹窗 -->
    <n-modal 
      v-model:show="showLogModal" 
      preset="card" 
      :title="`${currentApp?.name} - 实时日志`"
      style="width: 800px; max-width: 90vw;"
      :closable="true"
      @close="closeLogModal"
    >
      <RealtimeLogs 
        v-if="showLogModal && currentApp"
        :app-id="currentApp.id"
        :app-name="currentApp.name"
        :is-dialog="true"
        @close="closeLogModal"
      />
      <template #footer>
        <n-button @click="closeLogModal">关闭</n-button>
      </template>
    </n-modal>

    <!-- 进程排行弹窗（CPU/内存） -->
    <n-modal 
      v-model:show="showProcessModal" 
      preset="card" 
      :title="processModalTitle"
      style="width: 700px; max-width: 90vw;"
      :closable="true"
    >
      <div class="process-modal-content">
        <n-spin :show="processLoading" size="medium">
          <n-table :bordered="false" class="process-table" v-if="processList.length > 0 && !processLoading">
            <thead>
              <tr>
                <th style="width: 15%">PID</th>
                <th style="width: 50%">进程名称</th>
                <th style="width: 20%; text-align: right">{{ processModalType === 'cpu' ? 'CPU%' : '内存%' }}</th>
                <th style="width: 15%; text-align: right">内存(MB)</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="proc in processList" :key="proc.pid" class="process-row">
                <td class="process-pid">{{ proc.pid }}</td>
                <td class="process-name" :title="proc.name">{{ proc.name }}</td>
                <td class="process-percent" :class="{ 'cpu-high': proc.cpu_percent > 50, 'mem-high': proc.mem_percent > 50 }">
                  {{ proc.cpu_percent?.toFixed(1) || proc.mem_percent?.toFixed(1) || '-' }}%
                </td>
                <td class="process-memory">{{ (proc.memory_mb || 0).toFixed(1) }}</td>
              </tr>
            </tbody>
          </n-table>
          <div v-else-if="!processLoading" class="process-empty">
            <span>暂无进程数据</span>
          </div>
        </n-spin>
      </div>
      <template #footer>
        <n-button @click="showProcessModal = false">关闭</n-button>
      </template>
    </n-modal>

    <!-- 磁盘清理建议弹窗 -->
    <n-modal 
      v-model:show="showDiskModal" 
      preset="card" 
      title="磁盘清理建议"
      style="width: 700px; max-width: 90vw;"
      :closable="true"
    >
      <div class="disk-modal-content">
        <n-spin :show="diskLoading" size="medium">
          <div v-if="diskSuggestions.length > 0 && !diskLoading" class="suggestions-list">
            <div 
              v-for="(item, index) in diskSuggestions" 
              :key="index" 
              class="suggestion-item"
            >
              <div class="suggestion-header">
                <span class="suggestion-icon">📁</span>
                <span class="suggestion-path">{{ item.path }}</span>
                <n-tag size="small" :color="getSizeTagColor(item.size_mb)" border-style="none">
                  {{ formatSize(item.size_mb) }}
                </n-tag>
              </div>
              <div class="suggestion-desc">{{ item.reason || item.description || '建议清理以释放磁盘空间' }}</div>
              <div class="suggestion-action" v-if="item.can_clear !== false">
                <n-button size="small" @click="clearFile(item.path)" :loading="clearingFile === item.path">
                  清空文件
                </n-button>
              </div>
            </div>
          </div>
          <div v-else-if="!diskLoading" class="disk-empty">
            <span>暂无磁盘清理建议</span>
          </div>
        </n-spin>
      </div>
      <template #footer>
        <n-button @click="showDiskModal = false">关闭</n-button>
        <n-button type="primary" @click="refreshDiskSuggestions" :loading="diskLoading">刷新</n-button>
      </template>
    </n-modal>

  </div>

</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, reactive } from 'vue'
import RealtimeLogs from '../components/RealtimeLogs.vue'
import axios from 'axios'
import { 
  NGrid, NGi, NProgress, NTag, NButton, NInput, NTable, NForm, NFormItem, NCard, NModal, NSpin, NSelect
} from 'naive-ui'

// ==================== Token 管理 ====================
const TOKEN_KEY = 'tatai_auth_token'

const getToken = () => localStorage.getItem(TOKEN_KEY)

// 带认证的请求封装
const authAxios = axios.create()

authAxios.interceptors.request.use(config => {
  const token = getToken()
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

authAxios.interceptors.response.use(
  response => response,
  error => {
    if (error.response && error.response.status === 401) {
      localStorage.removeItem(TOKEN_KEY)
      localStorage.removeItem('tatai_user_info')
      window.location.reload()
    }
    return Promise.reject(error)
  }
)
// =================================================

const apps = ref([])
const sysStats = ref({ cpu_percent: 0, mem_percent: 0, disk_percent: 0 })
const searchQuery = ref('')
const showAddModal = ref(false)
const addForm = reactive({ command: '' })
const emit = defineEmits(['switch-tab'])

const isInitialLoading = ref(true)
const isRefreshing = ref(false)

// 刷新控制
const refreshInterval = ref(10)
const refreshOptions = [
  { label: '5s', value: 5 },
  { label: '10s', value: 10 },
  { label: '30s', value: 30 },
  { label: '60s', value: 60 },
  { label: 'Off', value: 0 }
]

let timer = null

const stopTimer = () => {
  if (timer) {
    clearInterval(timer)
    timer = null
  }
}

const startTimer = () => {
  stopTimer()
  if (refreshInterval.value > 0) {
    timer = setInterval(refreshAll, refreshInterval.value * 1000)
  }
}

const handleIntervalChange = () => {
  window.$message?.info(`Refresh rate set to ${refreshInterval.value}s`)
  startTimer()
}

// 进程排行弹窗
const showProcessModal = ref(false)
const processLoading = ref(false)
const processList = ref([])
const processModalType = ref('cpu')
const processModalTitle = computed(() => processModalType.value === 'cpu' ? 'CPU 占用排行 (Top 5)' : '内存占用排行 (Top 5)')

// 磁盘清理建议弹窗
const showDiskModal = ref(false)
const diskLoading = ref(false)
const diskSuggestions = ref([])
const clearingFile = ref(null)

const showLogModal = ref(false)
const currentApp = ref(null)

const openLogModal = (app) => {
  currentApp.value = app
  showLogModal.value = true
}

const closeLogModal = () => {
  showLogModal.value = false
  currentApp.value = null
}

const openProcessRanking = async (type) => {
  processModalType.value = type
  showProcessModal.value = true
  await fetchTopProcesses(type)
}

const fetchTopProcesses = async (type) => {
  processLoading.value = true
  try {
    const response = await authAxios.get(`/sys/top-processes?type=${type}`)
    if (response.data && Array.isArray(response.data)) {
      processList.value = response.data
    } else {
      processList.value = []
    }
  } catch (error) {
    console.error('获取进程排行失败:', error)
    processList.value = []
  } finally {
    processLoading.value = false
  }
}

const openDiskSuggestions = async () => {
  showDiskModal.value = true
  await fetchDiskSuggestions()
}

const fetchDiskSuggestions = async () => {
  diskLoading.value = true
  try {
    const response = await authAxios.get('/sys/disk-suggestions')
    if (response.data && Array.isArray(response.data)) {
      diskSuggestions.value = response.data
    } else {
      diskSuggestions.value = []
    }
  } catch (error) {
    console.error('获取磁盘建议失败:', error)
    diskSuggestions.value = []
  } finally {
    diskLoading.value = false
  }
}

const refreshDiskSuggestions = () => {
  fetchDiskSuggestions()
}

const clearFile = async (filePath) => {
  clearingFile.value = filePath
  try {
    await authAxios.post('/sys/clear-file', { path: filePath })
    await fetchDiskSuggestions()
  } catch (error) {
    console.error('清空文件失败:', error)
    alert('清空文件失败: ' + (error.response?.data || error.message))
  } finally {
    clearingFile.value = null
  }
}

const formatSize = (sizeMB) => {
  if (sizeMB >= 1024) {
    return (sizeMB / 1024).toFixed(2) + ' GB'
  }
  return sizeMB.toFixed(2) + ' MB'
}

const getSizeTagColor = (sizeMB) => {
  if (sizeMB > 1024) return { color: '#7f1d1d', textColor: '#fca5a5' }
  if (sizeMB > 512) return { color: '#78350f', textColor: '#fbbf24' }
  if (sizeMB > 100) return { color: '#14532d', textColor: '#4ade80' }
  return { color: '#1e293b', textColor: '#94a3b8' }
}

const getAppIcon = (type) => {
  switch (type) {
    case 'docker': return '🐳'
    case 'jar': return '☕'
    case 'nginx': return '🚀'
    default: return '📦'
  }
}

const getAppTypeLabel = (type) => {
  switch (type) {
    case 'docker': return 'Docker'
    case 'jar': return 'Java Jar'
    case 'nginx': return 'Nginx'
    default: return 'Other'
  }
}

const getTypeTagColor = (type) => {
  switch (type) {
    case 'docker': return { color: '#064e3b', textColor: '#10b981' }
    case 'jar': return { color: '#312e81', textColor: '#818cf8' }
    case 'nginx': return { color: '#14532d', textColor: '#4ade80' }
    default: return { color: '#78350f', textColor: '#fbbf24' }
  }
}

const sysMetrics = computed(() => [
  { 
    label: 'CPU', 
    shortLabel: 'CPU', 
    val: sysStats.value.cpu_percent, 
    color: '#3b82f6', 
    actionLabel: 'Occupancy ranking', 
    action: () => openProcessRanking('cpu') 
  },
  { 
    label: 'RAM', 
    shortLabel: 'RAM', 
    val: sysStats.value.mem_percent, 
    color: '#a855f7', 
    actionLabel: 'Occupancy ranking', 
    action: () => openProcessRanking('mem') 
  },
  { 
    label: 'DISK', 
    shortLabel: 'DISK', 
    val: sysStats.value.disk_percent, 
    color: '#f59e0b', 
    actionLabel: 'Cleanup suggestions', 
    action: openDiskSuggestions 
  }
])

const filteredApps = computed(() => apps.value.filter(a => (a.name || '').includes(searchQuery.value)))

const refreshAll = async () => {
  isRefreshing.value = true
  try {
    const [a, s] = await Promise.all([
      authAxios.get('/apps'), 
      authAxios.get('/sys/stats')
    ])
    apps.value = a.data || []
    sysStats.value = s.data || sysStats.value
  } catch (e) { 
    console.error(e) 
  } finally {
    isInitialLoading.value = false
    isRefreshing.value = false
  }
}

const handleAction = async (id, act) => {
  await authAxios.post(`/apps/${id}/${act}`)
  refreshAll()
}

const confirmAddApp = () => { showAddModal.value = false }

onMounted(() => {
  refreshAll()
  startTimer()
})

onUnmounted(() => {
  stopTimer()
})
</script>


<style scoped>

.refresh-controls {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 12px;
  margin-bottom: 15px;
}

.refresh-label {
  color: #64748b;
  font-size: 13px;
}

/* 调整选择器在暗色背景下的样式（如果需要微调） */
:deep(.n-base-selection) {
  --n-border: 1px solid #1e293b !important;
  background-color: #0f172a !important;
}

.dashboard-wrapper { width: 60%; min-width: 1150px; margin: 0 auto; }
.toolbar-row { display: flex; justify-content: space-between; margin-bottom: 30px; }
.tatai-btn-mint { background-color: #5eead4 !important; color: #0f172a !important; font-weight: bold; }
.tatai-search { width: 320px; }

.metric-card-v2 { 
  background: #112240; 
  border: 1px solid #1e293b; 
  border-radius: 12px; 
  padding: 24px; 
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 200px;
}
.progress-box {
  display: flex;
  justify-content: center;
  align-items: center;
  margin: 15px 0;
  width: 100%;
}
.metric-title {
  text-align: center;
  width: 100%;
}

.metric-btn {
  margin-top: auto;
  display: flex;
  justify-content: center;
}

.section-container { background: #112240; border: 1px solid #1e293b; border-radius: 12px; padding: 24px; margin: 24px 0; }
.section-title { margin-bottom: 20px; font-size: 15px; font-weight: bold; color: #f1f5f9; display: flex; align-items: center; gap: 8px; }
.service-card-item { background: #0f172a !important; border: 1px solid #1e293b; border-radius: 10px; padding: 18px; }
.card-header-top { display: flex !important; justify-content: space-between !important; align-items: center !important; width: 100%; margin-bottom: 12px; }
.service-info-box { display: flex !important; align-items: center !important; gap: 12px; }
.service-names h4 { margin: 0 !important; line-height: 1.2; font-size: 15px; color: #f8fafc; }
.service-names .sub-label { display: block; font-size: 11px; color: #64748b; }
.app-icon { background: #1e293b; padding: 8px; border-radius: 6px; font-size: 18px; flex-shrink: 0; }
.status-bar-dark { background: #1e293b; padding: 10px 15px; border-radius: 6px; margin: 15px 0; display: flex; align-items: center; gap: 10px; color: #94a3b8; }
.dot { width: 8px; height: 8px; border-radius: 50%; background: #475569; }
.dot.running { background: #10b981; }
.card-actions { display: flex; gap: 10px; }
.btn-start { flex: 1; background: #5eead4 !important; color: #0f172a !important; }
.btn-logs { flex: 1; border: 1px solid #334155 !important; color: #fff !important; }
.bottom-section { margin-top: 40px; }
.table-card-v2 { background: #112240 !important; border-radius: 12px; padding: 15px; }
.tatai-design-table :deep(th) { background-color: transparent !important; color: #64748b !important; font-size: 13px; padding: 12px 20px; }
.table-row-styled td { background-color: #0f172a !important; border-bottom: 4px solid #112240 !important; padding: 16px 20px !important; }
.name-cell { display: flex; align-items: center; gap: 15px; }
.type-icon-wrapper { background: #1e293b; width: 32px; height: 32px; display: flex; align-items: center; justify-content: center; border-radius: 6px; font-size: 16px; }
.app-name-text { color: #f8fafc; font-weight: 500; font-size: 14px; }
.status-indicator-inline { display: flex; align-items: center; justify-content: flex-end; gap: 8px; }
.status-dot { width: 8px; height: 8px; border-radius: 50%; background: #475569; }
.status-dot.is-running { background: #10b981; box-shadow: 0 0 8px #10b981; }
.status-text-val { color: #94a3b8; font-size: 13px; }

.empty-data-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 40px 0;
}
.empty-svg {
  height: 180px;
  width: auto;
  opacity: 0.6;
  margin-bottom: 16px;
}
.empty-text {
  color: #64748b;
  font-size: 14px;
}
.go-link {
  margin-top: 10px;
  text-decoration: underline;
  font-size: 14px;
}

.btn-start.btn-stop {
  background: #ef4444 !important;
  color: white !important;
}
.btn-start.btn-stop:hover {
  background: #dc2626 !important;
}

/* 进程排行弹窗样式 - 使用独立类名避免干扰 */
.process-modal-content {
  min-height: 300px;
  max-height: 500px;
  overflow-y: auto;
}
.process-table {
  width: 100%;
}
.process-table :deep(th) {
  background-color: #0f172a !important;
  color: #94a3b8 !important;
  font-size: 13px;
  padding: 12px 16px !important;
  border-bottom: 1px solid #1e293b !important;
}
.process-row td {
  background-color: transparent !important;
  border-bottom: 1px solid #1e293b !important;
  padding: 10px 16px !important;
  font-size: 13px;
}
.process-pid {
  color: #64748b;
  font-family: monospace;
}
.process-name {
  color: #f1f5f9;
  word-break: break-all;
  max-width: 300px;
  overflow: hidden;
  text-overflow: ellipsis;
}
.process-percent {
  text-align: right;
  font-family: monospace;
}
.process-percent.cpu-high {
  color: #f97316;
  font-weight: bold;
}
.process-percent.mem-high {
  color: #a855f7;
  font-weight: bold;
}
.process-memory {
  text-align: right;
  color: #94a3b8;
  font-family: monospace;
}
.process-empty {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 200px;
  color: #64748b;
}

/* 磁盘清理建议弹窗样式 */
.disk-modal-content {
  min-height: 300px;
  max-height: 500px;
  overflow-y: auto;
}
.suggestions-list {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.suggestion-item {
  background: #0f172a;
  border: 1px solid #1e293b;
  border-radius: 8px;
  padding: 12px 16px;
}
.suggestion-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 8px;
}
.suggestion-icon {
  font-size: 18px;
}
.suggestion-path {
  flex: 1;
  color: #f1f5f9;
  font-size: 13px;
  font-family: monospace;
  word-break: break-all;
}
.suggestion-desc {
  color: #94a3b8;
  font-size: 12px;
  margin-bottom: 12px;
  padding-left: 30px;
}
.suggestion-action {
  padding-left: 30px;
}
.disk-empty {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 200px;
  color: #64748b;
}

.section-container, .table-card-v2 {
  transition: opacity 0.3s ease;
}

.loading-container {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 200px;
}
</style>