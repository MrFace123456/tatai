<template>
  <div class="settings-container">
    <n-space vertical size="large">
      <div class="view-header">
        <div class="view-title-group">
          <h2 class="view-title">SYSTEM SETTINGS</h2>
          <span class="view-subtitle">Access Control & User Management</span>
        </div>
      </div>

      <n-card title="USER MANAGEMENT" :bordered="false" class="tatai-card">
        <n-data-table
          :columns="columns"
          :data="userList"
          :pagination="pagination"
          :bordered="false"
          class="tatai-table"
        />
        <div style="display: flex; justify-content: flex-end; margin-top: 16px;">
          <n-button type="primary" ghost @click="showCreateModal = true">
            <template #icon><span>+</span></template>
            NEW OPERATOR
          </n-button>
        </div>
      </n-card>

      <n-card title="NOTIFICATION CONFIGURATION" :bordered="false" class="tatai-card">
        <n-form label-placement="top" class="tatai-form">
          <n-form-item label="CHANNEL TYPE">
            <n-tag :bordered="false" type="primary" color="#1a2c4e" text-color="#5eead4">
              WEBHOOK
            </n-tag>
          </n-form-item>
          <n-form-item label="ENDPOINT URL">
            <n-input 
              v-model:value="webhookConfig.url" 
              placeholder="https://hooks.example.com/services/..."
            />
          </n-form-item>
          
          <div style="display: flex; justify-content: flex-end; gap: 12px; margin-top: 10px;">
            <n-button quaternary @click="handleTestWebhook">
              TEST SIGNAL
            </n-button>
            <n-button type="primary" ghost @click="handleSaveWebhook" style="width: 120px;">
              SAVE CONFIG
            </n-button>
          </div>
        </n-form>
      </n-card>
    </n-space>

    <n-modal v-model:show="showCreateModal" preset="card" title="INITIALIZE NEW USER" class="tatai-modal" style="width: 800px;">
      <n-form :model="newUserForm" label-placement="top">
        <n-form-item label="IDENTIFIER (Username)">
          <n-input v-model:value="newUserForm.username" placeholder="Enter username" />
        </n-form-item>
        <n-form-item label="INITIAL ACCESS TOKEN (Password)">
          <n-input v-model:value="newUserForm.password" type="password" placeholder="Set password" />
        </n-form-item>
      </n-form>
      <template #footer>
        <n-button block color="#5eead4" @click="handleCreateUser">AUTHORIZE CREATION</n-button>
      </template>
    </n-modal>

    <n-modal v-model:show="showPasswordModal" preset="card" title="RESET ACCESS TOKEN" class="tatai-modal" style="width: 600px;">
      <n-form-item label="NEW PASSWORD">
        <n-input v-model:value="passwordUpdate.newPassword" type="password" />
      </n-form-item>
      <template #footer>
        <n-button block color="#5eead4" @click="handlePasswordUpdate">UPDATE TOKEN</n-button>
      </template>
    </n-modal>
  </div>
</template>

<script setup>
import { ref, h, reactive, onMounted } from 'vue'
import { 
  NSpace, NCard, NDataTable, NButton, NTag, NSwitch, 
  NModal, NForm, NFormItem, NInput, useMessage 
} from 'naive-ui'
import axios from 'axios'

const message = useMessage()

// Token 管理
const TOKEN_KEY = 'tatai_auth_token'
const getToken = () => localStorage.getItem(TOKEN_KEY)

// 认证请求封装
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
      window.location.href = '/'
      message.error('登录已过期，请重新登录')
    }
    return Promise.reject(error)
  }
)

// 状态管理
const showCreateModal = ref(false)
const showPasswordModal = ref(false)
const targetUser = ref(null)
const loading = ref(false)

const newUserForm = reactive({
  username: '',
  password: ''
})

const passwordUpdate = reactive({
  newPassword: ''
})

const userList = ref([])
// 通知状态
const webhookConfig = reactive({
  url: '',
  events: ["app_crash"] // 默认开启应用崩溃通知
})

// 获取 Webhook 配置
const fetchWebhook = async () => {
  try {
    const res = await authAxios.get('/sys/webhook')
    if (res.data && res.data.url) {
      webhookConfig.url = res.data.url
    }
  } catch (error) {
    console.error('Failed to load webhook config')
  }
}

// 保存 Webhook
const handleSaveWebhook = async () => {
  if (!webhookConfig.url) {
    message.warning('Please enter a valid webhook URL')
    return
  }
  try {
    await authAxios.post('/sys/webhook', {
      url: webhookConfig.url,
      events: webhookConfig.events
    })
    message.success('Notification channel updated')
  } catch (error) {
    message.error('Failed to save notification settings')
  }
}

// 测试 Webhook
const handleTestWebhook = async () => {
  try {
    await authAxios.post('/test/webhook', {
      title: 'TATAI TEST NODE',
      content: 'Connectivity test from system settings.'
    })
    message.info('Test signal dispatched')
  } catch (error) {
    message.error('Test dispatch failed')
  }
}


// 获取用户列表
const fetchUsers = async () => {
  loading.value = true
  try {
    const res = await authAxios.get('/users')
    userList.value = res.data
  } catch (error) {
    message.error('获取用户列表失败')
  } finally {
    loading.value = false
  }
}


// 创建用户
const handleCreateUser = async () => {
  if (!newUserForm.username || !newUserForm.password) {
    message.warning('Incomplete credentials')
    return
  }
  try {
    await authAxios.post('/users', {
      username: newUserForm.username,
      password: newUserForm.password
    })
    message.success(`User ${newUserForm.username} initialized`)
    showCreateModal.value = false
    newUserForm.username = ''
    newUserForm.password = ''
    fetchUsers()
  } catch (error) {
    const errMsg = error.response?.data?.error || '创建用户失败'
    message.error(errMsg)
  }
}

// 修改用户状态
const handleStatusChange = async (row, newStatus) => {
  try {
    await authAxios.put(`/users/${row.id}/status`, { status: newStatus ? 1 : 0 })
    message.info(`${row.username} status updated to ${newStatus ? 'ENABLED' : 'DISABLED'}`)
    fetchUsers() // 刷新列表
  } catch (error) {
    const errMsg = error.response?.data?.error || '修改状态失败'
    message.error(errMsg)
    // 恢复原状态（前端已先改变，需要回滚）
    row.status = !newStatus
    fetchUsers()
  }
}

// 重置密码（弹窗）
const openResetPassword = (row) => {
  targetUser.value = row
  passwordUpdate.newPassword = ''
  showPasswordModal.value = true
}

// 执行重置密码
const handlePasswordUpdate = async () => {
  if (!passwordUpdate.newPassword) {
    message.warning('Please enter new password')
    return
  }
  try {
    await authAxios.put(`/admin/users/${targetUser.value.id}/password`, {
      new_password: passwordUpdate.newPassword
    })
    message.success('Security token synchronized')
    showPasswordModal.value = false
    targetUser.value = null
    passwordUpdate.newPassword = ''
  } catch (error) {
    const errMsg = error.response?.data?.error || '重置密码失败'
    message.error(errMsg)
  }
}

// 表格列定义
const columns = [
  { title: 'IDENTIFIER', key: 'username', render(row) {
    return h('span', { style: 'font-weight: bold; color: #5eead4' }, row.username)
  }},
  { 
    title: 'STATUS', 
    key: 'status', 
    render(row) {
      const isAdmin = row.username === 'admin'
      return h(NSwitch, {
        value: row.status === 1,
        disabled: isAdmin,
        'onUpdate:value': (val) => handleStatusChange(row, val)
      })
    }
  },
  { title: 'LAST ACCESS', key: 'lastLogin', render(row) {
    return row.lastLogin || 'N/A'
  }},
  {
    title: 'ACTIONS',
    key: 'actions',
    render(row) {
      return h(NSpace, {}, {
        default: () => [
          h(NButton, {
            size: 'small',
            quaternary: true,
            onClick: () => openResetPassword(row)
          }, { default: () => 'Reset Pwd' })
        ]
      })
    }
  }
]

const pagination = { pageSize: 10 }

onMounted(() => {
  fetchUsers()
  fetchWebhook()
})
</script>

<style scoped>
.settings-container {
  width: 60%;
  min-width: 1150px;
  margin: 0 auto;
  padding: 20px 0;
}

.view-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  margin-bottom: 10px;
  border-left: 4px solid #5eead4;
  padding-left: 20px;
}

.view-title {
  margin: 0;
  font-size: 24px;
  letter-spacing: 4px;
  color: #f8fafc;
}

.view-subtitle {
  font-size: 12px;
  color: #64748b;
  letter-spacing: 2px;
  text-transform: uppercase;
}

/* 契合系统风格的卡片 */
.tatai-card {
  background-color: #1a2c4e !important;
  border: 1px solid rgba(94, 234, 212, 0.1) !important;
}

:deep(.n-card-header__main) {
  color: #5eead4 !important;
  letter-spacing: 2px;
  font-size: 16px;
}

/* 表格样式深度适配 */
.tatai-table :deep(.n-data-table-th) {
  background-color: rgba(7, 13, 25, 0.5) !important;
  color: #64748b !important;
  font-size: 12px;
  letter-spacing: 1px;
}

.tatai-table :deep(.n-data-table-td) {
  background-color: transparent !important;
  color: #e2e8f0;
  border-bottom: 1px solid rgba(94, 234, 212, 0.05) !important;
}

/* 模态框样式 */
.tatai-modal {
  width: 500px;
  background-color: #0f172a !important;
  border: 1px solid #5eead4 !important;
}

:deep(.n-form-item-label) {
  color: #64748b !important;
  font-size: 12px;
  letter-spacing: 1px;
}
</style>