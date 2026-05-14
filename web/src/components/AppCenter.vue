<template>
  <div class="app-center-wrapper">
    <n-grid :cols="4" :x-gap="20" class="stats-row">
      <n-gi v-for="stat in summaryStats" :key="stat.label">
        <div class="stat-card-mini">
          <div class="stat-info">
            <div class="stat-label">{{ stat.label }}</div>
            <div class="stat-value" :style="{ color: stat.color }">{{ stat.value }}</div>
          </div>
          <div class="stat-icon-bg" :style="{ color: stat.color }">{{ stat.icon }}</div>
        </div>
      </n-gi>
    </n-grid>

    <div class="bottom-section">
      <div class="table-toolbar">
        <div class="section-title">
          <span class="header-icon">☕</span>
          <span class="header-text">App List</span>
        </div>
        <div class="toolbar-filters">
          <n-select v-model:value="typeFilter" :options="typeOptions" placeholder="All Types" class="filter-select-type" />
          <n-select v-model:value="statusFilter" :options="statusOptions" placeholder="All Status" class="filter-select-status" />
          <n-input v-model:value="filterQuery" placeholder="Search name..." clearable class="filter-input">
            <template #prefix>🔍</template>
          </n-input>
          <n-button type="primary" class="tatai-btn-mint" @click="handleOpenCreate">
            <template #icon>⚡</template>
            DEPLOY NEW
          </n-button>
        </div>
      </div>

      <n-card :bordered="false" class="table-card-v2">
        <n-table :bordered="false" class="tatai-design-table">
          <thead>
            <tr>
              <th style="width: 30%">Service Name</th>
              <th style="width: 15%; text-align: center">Type</th>
              <th style="width: 15%; text-align: center">Status</th>
              <th style="width: 20%">端口</th>
              <th style="width: 25%">Remark</th>
              <th style="width: 15%; text-align: right">Operations</th>
            </tr>
          </thead>
          <tbody>
            <tr 
              v-for="app in filteredApps" 
              :key="app.id" 
              class="table-row-styled clickable-row"
              :class="{ 'row-active': currentAppId === app.id }"
              @click="handleRowClick(app)"
            >
              <td class="name-cell">
                <span class="type-icon-wrapper">{{ getAppIcon(app.type) }}</span>
                <span class="app-name-text">{{ app.name }}</span>
              </td>
              <td style="text-align: center">
                <n-tag :color="getTypeTagColor(app.type)" border-style="none" size="small" round>{{ getAppTypeLabel(app.type) }}</n-tag>
              </td>
              <td style="text-align: center">
                <div class="status-indicator-inline">
                  <span class="status-dot" :class="{ 'is-running': app.status === 'running' }"></span>
                  <span class="status-text-val">{{ app.status === 'running' ? 'Running' : 'Stopped' }}</span>
                </div>
              </td>
              <td class="port-cell">
                <a 
                  href="javascript:void(0)" 
                  class="port-link"
                  @click.stop="showPortDetail(app)"
                >
                  {{ getPortDisplay(app) }}
                  <span 
                    v-if="app.ports_match_status === 'mismatch' && app.status === 'running'"
                    style="margin-left: 4px; font-size: 14px;"
                    title="端口与期望配置不匹配，点击查看详情"
                  >
                    ⚠️
                  </span>
                </a>
              </td>
              <td class="remark-cell">{{ app.remark || '-' }}</td>
              <td style="text-align: right;white-space: nowrap;" @click.stop>
                <n-button v-if="app.status === 'running'" text type="error" size="small" @click="handleStop(app)">Stop</n-button>
                <n-button v-else text type="success" size="small" @click="handleStart(app)">Start</n-button>
                <!-- 新增删除按钮 -->
                <n-button text type="error" size="small" @click="handleDelete(app)" style="margin-left: 8px" >Delete</n-button>
              </td>
            </tr>
          </tbody>
        </n-table>
      </n-card>
    </div>

    <n-drawer v-model:show="drawerVisible" :style="{
      width: 'min(40%, 800px)',
      minWidth: '450px'
      }" placement="right"
    >
      <n-drawer-content 
        :title="drawerMode === 'create' ? '🏗️ QUICK CREATE' : '🛠️ WORKSPACE CONTROL'" 
        closable
      >
        <n-tabs type="line" animated>
          <n-tab-pane name="config" tab="⚙️ Configuration">
            <n-form :model="createForm" label-placement="top" size="small" style="margin-top: 15px">
              <n-form-item label="应用名称" required>
                <n-input v-model:value="createForm.appName" placeholder="输入应用显示名称" />
              </n-form-item>

              <n-form-item label="Service Type">
                <n-radio-group v-model:value="createForm.type" name="appType">
                  <n-radio-button value="docker">Docker</n-radio-button>
                  <n-radio-button value="nginx">Nginx</n-radio-button>
                  <n-radio-button value="jar">Java(Jar)</n-radio-button>
                  <n-radio-button value="other">Other</n-radio-button>
                </n-radio-group>
              </n-form-item>

              <div class="drawer-dynamic-scroll">
                <template v-if="createForm.type === 'docker'">
                  <n-form-item label="App Name / Container Name" required>
                    <n-input v-model:value="createForm.appName" placeholder="Input the application name(container name)" />
                  </n-form-item>
                  <n-form-item label="Docker Run Command">
                    <n-input v-model:value="createForm.cmd" type="textarea" :autosize="{ minRows: 3 }" />
                  </n-form-item>
                </template>

                <template v-if="createForm.type === 'nginx'">
                  <n-form-item label="Config Path">
                    <n-input v-model:value="createForm.nginxConf" />
                  </n-form-item>
                </template>

                <template v-if="createForm.type === 'jar'">
                  <n-form-item label="JDK">
                    <n-select v-model:value="createForm.jdkPath" :options="jdkOptions" />
                  </n-form-item>
                  <n-form-item label="Jar Path">
                    <n-input v-model:value="createForm.jarPath" />
                  </n-form-item>
                  <n-form-item label="期望端口">
                    <n-input 
                      v-model:value="createForm.ports" 
                      placeholder='JSON数组格式，如 [8080, 9090]'
                    />
                    <template #feedback>
                      <span style="font-size: 12px; color: #64748b;">格式示例：[8080, 9090]</span>
                    </template>
                  </n-form-item>
                  <n-form-item label="Daemon">
                    <n-switch v-model:value="createForm.isDaemon" />
                  </n-form-item>
                </template>

                <template v-if="createForm.type === 'other'">
                  <n-form-item label="Start Command">
                    <n-input v-model:value="createForm.startCmd" />
                  </n-form-item>
                  <n-form-item label="Check Command">
                    <n-input v-model:value="createForm.checkCmd" />
                  </n-form-item>
                  <n-form-item label="期望端口">
                    <n-input 
                      v-model:value="createForm.ports" 
                      placeholder='JSON数组格式，如 [8080, 9090]'
                    />
                    <template #feedback>
                      <span style="font-size: 12px; color: #64748b;">格式示例：[8080, 9090]</span>
                    </template>
                  </n-form-item>
                </template>
              </div>

              <div style="margin-top: 24px">
                <n-button type="primary" class="tatai-btn-mint" block @click="handleCreate">
                  {{ drawerMode === 'create' ? '⚡ DEPLOY' : '💾 SAVE CHANGES' }}
                </n-button>
              </div>
            </n-form>
          </n-tab-pane>

          <n-tab-pane name="logs" tab="📜 Real-time Logs" v-if="drawerMode === 'edit'">
            <div style="height: calc(100vh - 200px); background: #0a192f; border-radius: 8px; margin-top: 10px">
              <RealtimeLogs :app-id="currentAppId" :app-name="currentAppName" :show-header="false" />
            </div>
          </n-tab-pane>
        </n-tabs>
      </n-drawer-content>
    </n-drawer>
  </div>
  <!-- 端口详情弹窗 -->
  <n-modal v-model:show="portModalVisible" preset="dialog" title="端口详情">
    <div v-if="currentPortApp" class="port-detail-content">
      <div class="port-detail-item">
        <strong>应用名称：</strong> {{ currentPortApp.name }}
      </div>
      <div class="port-detail-item">
        <strong>应用类型：</strong> {{ getAppTypeLabel(currentPortApp.type) }}
      </div>
      <div class="port-detail-item">
        <strong>期望端口：</strong>
        <span v-if="currentPortApp.ports && currentPortApp.ports !== '[]'">
          {{ JSON.parse(currentPortApp.ports).join(', ') }}
        </span>
        <span v-else>未配置</span>
      </div>
      <!-- 统一显示实际端口（所有类型） -->
      <div class="port-detail-item">
        <strong>实际监听端口：</strong>
        <span v-if="currentPortApp.status === 'running'">
          <template v-if="currentPortApp.actual_ports && currentPortApp.actual_ports.length > 0">
            <span>{{ currentPortApp.actual_ports.filter(p => p !== 0).join(', ') || '无有效端口' }}</span>
          </template>
          <span v-else>获取中或未捕获到端口</span>
        </span>
        <span v-else>应用未运行</span>
      </div>
      <div v-if="currentPortApp.ports_match_status === 'mismatch'" class="port-detail-warning">
        ⚠️ 实际端口与期望端口不匹配
      </div>
    </div>
    <template #action>
      <n-button @click="portModalVisible = false">关闭</n-button>
    </template>
  </n-modal>
</template>

<script setup>
import { ref, reactive, computed, onMounted } from 'vue'
import axios from 'axios'
import RealtimeLogs from '../components/RealtimeLogs.vue'
import {
  NGrid,
  NGi,
  NForm,
  NFormItem,
  NInput,
  NButton,
  NRadioGroup,
  NRadioButton,
  NTag,
  NTable,
  NCard,
  NDivider,
  NSelect,
  NInputGroup,
  NSwitch,
  NDrawer,
  NDrawerContent,
  NTabs,
  NTabPane,
  useMessage,
  NModal 
} from 'naive-ui'

const message = useMessage()

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
      // Token 过期，清除本地登录状态并跳转
      localStorage.removeItem(TOKEN_KEY)
      localStorage.removeItem('tatai_user_info')
      window.location.reload()
      message.error('登录已过期，请重新登录')
    }
    return Promise.reject(error)
  }
)
// =================================================

// --- 状态控制 ---
const drawerVisible = ref(false)
const drawerMode = ref('create')
const loading = ref(false)
const jdkLoading = ref(false)
const jdkList = ref([])
const apps = ref([])
const currentAppId = ref(null)
const currentAppName = ref('')

// --- 筛选状态 ---
const typeFilter = ref(null)
const filterQuery = ref('')
const statusFilter = ref(null)

const typeOptions = [
  { label: 'All Types', value: null },
  { label: 'Docker', value: 'docker' },
  { label: 'Java (JAR)', value: 'jar' },
  { label: 'Nginx', value: 'nginx' },
  { label: 'Other', value: 'other' }
]
const statusOptions = [
  { label: 'All Status', value: null },
  { label: 'Running', value: 'running' },
  { label: 'Stopped', value: 'stopped' }
]

// --- 表单对象 ---
const createForm = reactive({
  type: 'docker',
  appName: '',
  cmd: '',
  dockerName: '',
  nginxConf: '',
  nginxExec: 'nginx',
  jdkPath: null,
  jarPath: '',
  isDaemon: true,
  startCmd: '',
  stopCmd: '',
  checkCmd: '',
  ports: '' 
})

// --- 核心方法 ---

const resetForm = () => {
  createForm.type = 'docker'
  createForm.appName = ''
  createForm.cmd = ''
  createForm.dockerName = ''
  createForm.nginxConf = ''
  createForm.nginxExec = 'nginx'
  createForm.jdkPath = null
  createForm.jarPath = ''
  createForm.isDaemon = true
  createForm.startCmd = ''
  createForm.stopCmd = ''
  createForm.checkCmd = ''
  currentAppId.value = null
  currentAppName.value = ''
  createForm.ports = ''
}

const handleOpenCreate = () => {
  drawerMode.value = 'create'
  resetForm()
  drawerVisible.value = true
}

const handleRowClick = (app) => {
  drawerMode.value = 'edit'
  currentAppId.value = app.id
  currentAppName.value = app.name

  createForm.type = app.type || 'other'
  createForm.appName = app.name || ''
  createForm.cmd = app.command || ''
  createForm.dockerName = app.docker_name || ''
  createForm.nginxConf = app.nginx_path || ''
  createForm.nginxExec = 'nginx'
  createForm.jdkKey = app.jdk_key || null
  createForm.jarPath = app.jar_path || ''
  createForm.isDaemon = (app.is_daemon !== undefined) ? app.is_daemon : true
  createForm.startCmd = app.command || ''
  createForm.stopCmd = app.stop_cmd || ''
  createForm.checkCmd = app.check_cmd || ''
  createForm.ports = (app.ports && app.ports !== '[]') ? app.ports : ''

  drawerVisible.value = true
}

const fetchJdkList = async () => {
  jdkLoading.value = true
  try {
    const response = await authAxios.get('/jdk/list')
    jdkList.value = response.data || []
  } catch (error) {
    message.error('获取 JDK 列表失败')
  } finally {
    jdkLoading.value = false
  }
}

const jdkOptions = computed(() => {
  return jdkList.value.map(jdk => ({
    label: jdk.key,
    value: jdk.path
  }))
})

const fetchApps = async () => {
  loading.value = true
  try {
    const response = await authAxios.get('/apps')
    apps.value = response.data || []
  } catch (error) {
    message.error('获取应用列表失败')
  } finally {
    loading.value = false
  }
}

// 弹窗相关
const portModalVisible = ref(false)
const currentPortApp = ref(null)

const showPortDetail = (app) => {
  currentPortApp.value = app
  portModalVisible.value = true
}

const getPortDisplay = (app) => {
  if ((app.type === 'jar' || app.type === 'other') && app.status === 'running') {
    if (app.actual_ports && app.actual_ports.length > 0) {
      const validPorts = app.actual_ports.filter(p => p !== 0)
      if (validPorts.length > 0) {
        let portStr = validPorts.join(', ')
        if (portStr.length > 5) {
          return portStr.substring(0, 5) + '...'
        }
        return portStr
      }
    }
    return '未监听'
  }

  let portStr = ''
  if (app.type === 'docker') {
    if (app.actual_ports && app.actual_ports.length > 0) {
      portStr = app.actual_ports.join(', ')
    } else if (app.ports && app.ports !== '[]') {
      try {
        const ports = JSON.parse(app.ports)
        portStr = ports.join(', ')
      } catch (e) {
        portStr = '未配置'
      }
    } else {
      portStr = '未配置'
    }
  } else {
    if (app.ports && app.ports !== '[]') {
      try {
        const ports = JSON.parse(app.ports)
        portStr = ports.join(', ')
        if (portStr.length > 5) return portStr.substring(0, 5) + '...'
      } catch (e) {
        portStr = '未配置'
      }
    } else {
      portStr = '未配置'
    }
  }
  return portStr
}

const handleCreate = async () => {
  if (!createForm.appName || !createForm.appName.trim()) {
    message.warning('请输入应用名称')
    return
  }
  // Docker 类型：将 appName 同步到 dockerName
  if (createForm.type === 'docker') {
    createForm.dockerName = createForm.appName
  }
  // Docker、Nginx、Other 类型暂不调用后端，弹出开发中提示
  if (createForm.type === 'docker' || createForm.type === 'nginx' || createForm.type === 'other') {
    message.info('In the process of feature development, please stay tuned')
    return
  }


  try {
    if (drawerMode.value === 'edit') {
      await authAxios.put(`/apps/${currentAppId.value}`, createForm)
      message.success('更新成功')
    } else {
      const res = await authAxios.post('/apps', createForm)
      message.success('创建成功')
      currentAppId.value = res.data.id
      currentAppName.value = createForm.appName
      drawerMode.value = 'edit'
    }
    await fetchApps()
  } catch (error) {
    message.error('操作失败')
  }
}

const handleStart = async (app) => {
  try {
    await authAxios.post(`/apps/${app.id}/start`)
    message.success(`${app.name} 已启动`)
    await fetchApps()
  } catch (error) {
    message.error('启动失败')
  }
}

const handleStop = async (app) => {
  try {
    await authAxios.post(`/apps/${app.id}/stop`)
    message.success(`${app.name} 已停止`)
    await fetchApps()
  } catch (error) {
    message.error('停止失败')
  }
}

// 删除应用
const handleDelete = async (app) => {
  // 检查是否正在运行
  if (app.status === 'running') {
    message.warning('Please stop the application first')
    return
  }

  // 二次确认弹窗
  const confirmDelete = window.confirm(`Are you sure you want to delete the application "${app.name}" ? This operation cannot be undone.`)
  if (!confirmDelete) return

  try {
    await authAxios.delete(`/apps/${app.id}`)
    message.success(`${app.name} has been deleted`)
    
    // 刷新列表
    await fetchApps()
    await fetchStatistics()
    
    // 如果当前删除的应用正好是打开的抽屉中的，关闭抽屉
    if (drawerVisible.value && currentAppId.value === app.id) {
      drawerVisible.value = false
      currentAppId.value = null
    }
  } catch (error) {
    // 处理后端返回的错误信息
    const errorMsg = error.response?.data?.error || 'Deletion failed'
    if (errorMsg.includes('Please stop the application first')) {
      message.warning('Please stop the app before deleting it')
    } else {
      message.error(errorMsg)
    }
  }
}

// --- 统计数据区 ---
// 新增响应式变量
const statsData = ref({
  total: 0,
  running: 0,
  daemon: 0,
  abnormal: 0
})

// 获取统计数据的方法
const fetchStatistics = async () => {
  try {
    const response = await authAxios.get('/apps/summary')
    statsData.value = response.data
  } catch (error) {
    console.error('获取统计数据失败', error)
  }
}

// 修改 summaryStats 计算属性
const summaryStats = computed(() => [
  { label: 'Total Processes', value: statsData.value.total, color: '#5eead4', icon: '📦' },
  { label: 'Running Processes', value: statsData.value.running, color: '#10b981', icon: '⚡' },
  { label: 'Daemon Processes', value: statsData.value.daemon, color: '#a855f7', icon: '🛡️' },
  { label: 'Abnormal Processes', value: statsData.value.abnormal, color: '#f59e0b', icon: '⚠️' }
])

const filteredApps = computed(() => {
  return apps.value.filter(app => {
    const searchTerm = filterQuery.value.toLowerCase()
    const matchQuery = !searchTerm || (app.name && app.name.toLowerCase().includes(searchTerm))
    const matchStatus = statusFilter.value ? app.status === statusFilter.value : true
    const matchType = typeFilter.value ? app.type === typeFilter.value : true
    return matchQuery && matchStatus && matchType
  })
})

const getAppIcon = (type) => {
  const icons = { docker: '🐳', jar: '☕', nginx: '🚀' }
  return icons[type] || '📦'
}

const getAppTypeLabel = (type) => {
  const labels = { docker: 'Docker', jar: 'Java Jar', nginx: 'Nginx' }
  return labels[type] || 'Other'
}

const getTypeTagColor = (type) => {
  const colors = {
    docker: { color: '#064e3b', textColor: '#10b981' },
    jar: { color: '#312e81', textColor: '#818cf8' },
    nginx: { color: '#14532d', textColor: '#4ade80' }
  }
  return colors[type] || { color: '#78350f', textColor: '#fbbf24' }
}

onMounted(() => {
  fetchApps()
  fetchJdkList()
  fetchStatistics()
})
</script>

<style scoped>
/* ============================================================
   1. 头部统计区样式
   ============================================================ */
.stats-row {
  margin-bottom: 30px;
}

.stat-card-mini {
  background: rgba(17, 34, 64, 0.7);
  border: 1px solid #1e293b;
  border-radius: 12px;
  padding: 20px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  position: relative;
  overflow: hidden;
}

.stat-info {
  z-index: 2;
}

.stat-label {
  color: #94a3b8;
  font-size: 13px;
  margin-bottom: 4px;
}

.stat-value {
  font-size: 22px;
  font-weight: bold;
  font-family: 'Courier New', monospace;
}

.stat-icon-bg {
  font-size: 38px;
  opacity: 0.15;
  position: absolute;
  right: 10px;
  bottom: -5px;
  z-index: 1;
}

/* ============================================================
   2. 中间核心面板 (高度锁定 453px)
   ============================================================ */
.middle-section {
  margin-bottom: 30px;
}

.glass-panel.creation-panel,
.glass-panel.log-panel {
  background: rgba(17, 34, 64, 0.7);
  border: 1px solid #1e293b;
  border-radius: 12px;
  padding: 24px;
  min-height: 453px;
  max-height: 453px;
  display: flex;
  flex-direction: column;
}

.panel-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 20px;
  border-bottom: 1px solid #1e293b;
  padding-bottom: 12px;
  flex-shrink: 0;
}

.header-text {
  font-weight: bold;
  color: #f1f5f9;
  font-size: 16px;
}

/* ============================================================
   3. 左侧表单特有样式
   ============================================================ */
.tight-form {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.dynamic-fields-scroll {
  flex: 1;
  overflow-y: auto;
  padding-right: 4px;
  margin-top: 8px;
}

.form-actions {
  display: flex;
  gap: 12px;
  padding-top: 16px;
  border-top: 1px solid rgba(255, 255, 255, 0.05);
  flex-shrink: 0;
  margin-top: 12px;
}

.form-actions .n-button {
  flex: 1;
}

.full-width-radio {
  width: 100%;
}

.full-width-radio :deep(.n-radio-button) {
  flex: 1;
  text-align: center;
}

/* ============================================================
   4. 右侧日志占位样式
   ============================================================ */
.log-placeholder {
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #64748b;
  background: #0a192f;
  border-radius: 8px;
  margin-top: 10px;
}

/* ============================================================
   5. 底部列表与工具栏
   ============================================================ */
.bottom-section {
  margin-top: 30px;
}

.table-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

.toolbar-filters {
  display: flex;
  align-items: center;
  gap: 12px;
}

.filter-select-type {
  width: 140px;
}
.filter-select-status {
  width: 140px;
}
.filter-input {
  width: 220px;
}

.section-title {
  font-size: 15px;
  font-weight: bold;
  color: #f1f5f9;
  display: flex;
  align-items: center;
  gap: 8px;
}

.status-indicator-inline {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #475569;
}

.status-dot.is-running {
  background: #10b981;
  box-shadow: 0 0 8px #10b981;
}

.status-text-val {
  color: #94a3b8;
  font-size: 13px;
}

.empty-data-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 0;
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

/* ============================================================
   6. 通用组件样式
   ============================================================ */
.app-center-wrapper {
  width: 60%;
  min-width: 1150px;
  margin: 0 auto;
  padding-bottom: 60px;
}

.table-card-v2 {
  background: #112240 !important;
  border-radius: 12px;
}

.tatai-design-table :deep(th) {
  background-color: transparent !important;
  color: #64748b !important;
  font-size: 13px;
  padding: 12px 20px;
}

.table-row-styled td {
  background-color: #0f172a !important;
  border-bottom: 4px solid #112240 !important;
  padding: 16px 20px !important;
}

.tatai-btn-mint {
  background-color: #5eead4 !important;
  color: #0f172a !important;
  font-weight: bold;
}

.name-cell {
  display: flex;
  align-items: center;
  gap: 15px;
}

.type-icon-wrapper {
  background: #1e293b;
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 6px;
  font-size: 16px;
}

.app-name-text {
  color: #f8fafc;
  font-weight: 500;
  font-size: 14px;
}

.remark-cell {
  color: #94a3b8;
}

/* 自定义滚动条 */
::-webkit-scrollbar {
  width: 4px;
}
::-webkit-scrollbar-thumb {
  background: #1e293b;
  border-radius: 4px;
}

/* 覆盖折叠面板箭头与边框 */
:deep(.n-collapse-item__header) {
  padding: 15px 0 !important;
  border-bottom: 1px solid rgba(94, 234, 212, 0.1) !important;
}
:deep(.n-collapse-item__arrow) {
  color: #5eead4 !important;
  font-size: 20px !important;
  font-weight: bold;
}
:deep(.n-collapse-item__content-inner) {
  padding-top: 20px !important;
}

/* 抽屉与点击行联动样式 */
.clickable-row {
  cursor: pointer;
  transition: background-color 0.2s ease;
}

.clickable-row:hover {
  filter: brightness(1.1);
}

/* 优化后的行选中效果：整行边框高亮 */
.table-row-styled {
  position: relative;
  transition: all 0.2s ease;
}

/* 移除之前的竖线，改为整行外框高亮感 */
.table-row-styled.row-active td {
  background-color: rgba(94, 234, 212, 0.05) !important; /* 背景微亮 */
  border-top: 1px solid #5eead4 !important;
  border-bottom: 1px solid #5eead4 !important;
}

/* 选中行首尾单元格圆角处理，形成闭合框感 */
.table-row-styled.row-active td:first-child {
  border-left: 1px solid #5eead4 !important;
  border-top-left-radius: 8px;
  border-bottom-left-radius: 8px;
}

.table-row-styled.row-active td:last-child {
  border-right: 1px solid #5eead4 !important;
  border-top-right-radius: 8px;
  border-bottom-right-radius: 8px;
}

/* 悬停效果微调 */
.clickable-row:hover:not(.row-active) td {
  background-color: rgba(255, 255, 255, 0.02) !important;
}

.drawer-dynamic-scroll {
  max-height: calc(100vh - 450px);
  overflow-y: auto;
  padding-right: 4px;
}

/* 覆盖抽屉内部样式 */
:deep(.n-drawer-content) {
  background-color: #112240 !important;
}



.port-cell {
  color: #5eead4;
}
.port-link {
  color: #5eead4;
  text-decoration: none;
  cursor: pointer;
}
.port-link:hover {
  text-decoration: underline;
  opacity: 0.8;
}
.port-detail-content {
  padding: 8px 0;
}
.port-detail-item {
  margin-bottom: 12px;
  word-break: break-all;
}
.port-detail-warning {
  margin-top: 12px;
  padding: 8px;
  background-color: rgba(245, 158, 11, 0.2);
  border-radius: 4px;
  color: #f59e0b;
}


</style>