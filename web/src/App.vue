<template>
  <n-config-provider :theme="darkTheme" :theme-overrides="themeOverrides">
    <n-message-provider>
      <MessageApiInjecter />

      <template v-if="!isLoggedIn">
        <div class="login-container">
          <div class="login-card">
            <div class="login-header">
              <img src="/logo.png" class="login-logo" alt="TATAI" />
              <div class="login-title">TATAI SYSTEM</div>
              <div class="login-subtitle">TERMINAL ACCESS CONTROL</div>
            </div>
            
            <n-space vertical size="large">
              <n-input 
                v-model:value="loginForm.username" 
                type="text" 
                size="large"
                placeholder="Username / Identifier"
                class="tatai-login-input"
                :input-props="{ spellcheck: 'false' }"
              />
              <n-input 
                v-model:value="loginForm.password" 
                type="password" 
                size="large"
                show-password-on="mousedown"
                placeholder="Access Token / Password"
                class="tatai-login-input"
                @keyup.enter="handleLogin"
              />
              <n-button 
                block 
                strong 
                secondary
                size="large"
                color="#5eead4" 
                :loading="loading"
                class="login-submit-btn"
                @click="handleLogin"
              >
                AUTHORIZE & INITIALIZE
              </n-button>
            </n-space>
          </div>
        </div>
      </template>

      <template v-else>
        <n-layout class="tatai-root-layout" :native-scrollbar="false">
          <n-layout-header bordered class="tatai-header">
            <div class="header-content">
              <div class="header-left">
                <div class="logo-group">
                  <img src="/logo.png" class="logo-img" alt="TATAI" />
                  <span class="version-tag">v2.0.0</span>
                </div>
                <n-tabs type="line" class="nav-tabs" :value="activeMainTab" @update:value="activeMainTab = $event">
                  <n-tab name="dashboards">Dashboards</n-tab>
                  <n-tab name="app-center">App Center</n-tab>
                  <n-tab name="settings">Settings</n-tab>
                </n-tabs>
              </div>
              
              <div class="header-right">
                <n-button quaternary @click="handleLogout">
                  <template #icon>📤</template> Logout
                </n-button>
              </div>
            </div>
          </n-layout-header>

          <n-layout-content class="tatai-main-content">
            <Dashboard 
              v-if="activeMainTab === 'dashboards'" 
              @switch-tab="activeMainTab = $event" 
            />
            <AppCenter v-else-if="activeMainTab === 'app-center'" />
            <Settings v-else-if="activeMainTab === 'settings'" />
          </n-layout-content>
        </n-layout>

        <transition name="fade">
          <div v-if="showGuide" class="guide-overlay" @click="closeGuide">
            <div class="guide-content">
              <div class="mouse-icon">
                <div class="wheel"></div>
              </div>
              
              <h1 class="guide-tip status-glow">SYSTEM CHECK: ALL OPERATIONAL</h1>
              
              <p class="guide-sub-tip">FULL VIEW READY. SCROLL DOWN TO INSPECT ACTIVE INSTANCES.</p>
              
              <n-button 
                secondary 
                strong
                color="#5eead4" 
                class="guide-btn"
                @click="closeGuide"
              >
                INITIALIZE CONSOLE
              </n-button>
            </div>
          </div>
        </transition>
      </template>

    </n-message-provider>
  </n-config-provider>
</template>

<script setup>
import { ref, onMounted, reactive } from 'vue'
import { 
  darkTheme, useMessage,
  NConfigProvider, NMessageProvider, NLayout, NLayoutHeader, NLayoutContent,
  NBadge, NButton, NTabs, NTab, NInput, NSpace
} from 'naive-ui'

import Dashboard from './components/Dashboard.vue'
import AppCenter from './components/AppCenter.vue'
import Settings from './components/Settings.vue'

const MessageApiInjecter = {
  setup() {
    window.$message = useMessage()
    return () => null
  }
}

const themeOverrides = {
  common: {
    primaryColor: '#5eead4',
    primaryColorHover: '#2dd4bf',
    bodyColor: '#070d19',
    cardColor: '#1a2c4e',
    tableColor: 'transparent',
    textColorBase: '#f8fafc'
  }
}

const activeMainTab = ref('dashboards')
const showGuide = ref(false)

// Token 管理
const TOKEN_KEY = 'tatai_auth_token'
const USER_KEY = 'tatai_user_info'

const getToken = () => localStorage.getItem(TOKEN_KEY)
const setToken = (token) => localStorage.setItem(TOKEN_KEY, token)
const removeToken = () => localStorage.removeItem(TOKEN_KEY)

const getUserInfo = () => {
  const userStr = localStorage.getItem(USER_KEY)
  if (!userStr) return null
  try {
    return JSON.parse(userStr)
  } catch {
    return null
  }
}

const setUserInfo = (userInfo) => localStorage.setItem(USER_KEY, JSON.stringify(userInfo))
const removeUserInfo = () => localStorage.removeItem(USER_KEY)

// 检查登录状态
const checkLoginStatus = () => {
  const token = getToken()
  const userInfo = getUserInfo()
  if (token && userInfo) {
    isLoggedIn.value = true
  } else {
    isLoggedIn.value = false
  }
}

onMounted(() => {
  checkLoginStatus()
  
  // 如果已登录，不再显示引导
  if (isLoggedIn.value) {
    return
  }
  
  // 使用版本化的标记，方便未来更新引导
  const hasVisited = localStorage.getItem('tatai_v2_visited')
  if (!hasVisited) {
    showGuide.value = true
  }
})

const closeGuide = () => {
  showGuide.value = false
  localStorage.setItem('tatai_v2_visited', 'true')
}

// 登录相关逻辑
const isLoggedIn = ref(false)
const loading = ref(false)
const loginForm = reactive({
  username: '',
  password: ''
})

const handleLogin = async () => {
  if (!loginForm.username || !loginForm.password) {
    window.$message.warning('Please enter credentials')
    return
  }
  
  loading.value = true
  
  try {
    const response = await fetch('/api/auth/login', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        username: loginForm.username,
        password: loginForm.password
      })
    })
    
    const data = await response.json()
    
    if (!response.ok) {
      throw new Error(data.error || '登录失败')
    }
    
    // 保存Token和用户信息
    setToken(data.token)
    setUserInfo({
      username: data.username,
      role: data.role
    })
    
    isLoggedIn.value = true
    window.$message.success('Authorization Successful')
    
  } catch (e) {
    window.$message.error(e.message || 'Authorization Failed')
  } finally {
    loading.value = false
  }
}

const handleLogout = async () => {
  const token = getToken()
  if (token) {
    try {
      await fetch('/auth/logout', {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${token}`,
          'Content-Type': 'application/json',
        }
      })
    } catch (e) {
      console.warn('Logout request failed:', e)
    }
  }
  
  removeToken()
  removeUserInfo()
  isLoggedIn.value = false
  window.$message.success('Logged out successfully')
}
</script>

<style scoped>
/* 登录页新增样式 */
.login-container {
  height: 100vh;
  background-color: #0a192f;
  display: flex;
  justify-content: center;
  align-items: center;
  background-image: radial-gradient(circle at 50% 50%, rgba(94, 234, 212, 0.05) 0%, transparent 80%);
}

.login-card {
  width: 420px;
  padding: 50px 40px;
  background: #0f172a;
  border: 1px solid rgba(94, 234, 212, 0.2);
  border-radius: 2px;
  box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.5);
}

.login-header {
  text-align: center;
  margin-bottom: 40px;
}

.login-logo {
  height: 60px;
  margin-bottom: 16px;
  filter: drop-shadow(0 0 10px rgba(94, 234, 212, 0.3));
}

.login-title {
  color: #5eead4;
  font-size: 24px;
  font-weight: bold;
  letter-spacing: 6px;
  margin-left: 6px;
}

.login-subtitle {
  color: #64748b;
  font-size: 11px;
  letter-spacing: 3px;
  margin-top: 8px;
}

/* 彻底修复输入框：解决文字贴边和两端色差问题 */
.tatai-login-input :deep(.n-input-wrapper) {
  /* 强制设定背景颜色，确保整个输入框区域（包括插槽）颜色一致 */
  background-color: rgba(15, 23, 42, 1) !important;
  /* 增加内边距，解决文字紧贴左侧的问题 */
  padding-left: 16px !important;
  padding-right: 16px !important;
  border: 1px solid rgba(94, 234, 212, 0.1) !important;
}

/* 确保内部文字元素不会有自己的多余边距或背景 */
.tatai-login-input :deep(.n-input__input-el) {
  padding: 10px !important;
  height: 48px;
  line-height: 48px;
}

/* 移除密码图标区域可能存在的默认背景 */
.tatai-login-input :deep(.n-input__suffix) {
  background-color: transparent !important;
}

.login-submit-btn {
  height: 48px;
  font-weight: bold;
  letter-spacing: 2px;
  margin-top: 10px;
}

/* 1. 根布局：确保 body 不会出现双滚动条 */
.tatai-root-layout {
  min-height: 100vh;
  background-color: #0a192f !important;
}

/* 2. Header：精准添加冻结定位，背景色锁死为最初的 #0f172a */
:deep(.n-card) {
  border: 1px solid rgba(94, 234, 212, 0.1) !important; /* 给卡片加一个极细的青色边框 */
  box-shadow: 0 4px 20px -5px rgba(0, 0, 0, 0.5); /* 增加阴影深度 */
}
.tatai-header {
  /* --- 冻结核心新增代码 --- */
  position: fixed !important;
  top: 0;
  left: 0;
  width: 100%;
  z-index: 1000;
  /* ---------------------- */
  
  height: 90px;
  background-color: #0f172a !important; /* 维持最初版本背景色 */
  display: flex;
  align-items: center;
  border-bottom: 1px solid rgba(94, 234, 212, 0.2) !important;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
}

/* 3. 内容区：增加与 Header 高度一致的偏移，防止内容被遮挡 */
.tatai-main-content {
  background-color: #0a192f !important;
  padding: 20px 0;
  margin-top: 90px; /* 新增：对应 Header 的 90px 高度 */
}

/* --- 以下所有样式完全保留你最初版本的代码，未做任何改动 --- */
.header-content {
  width: 60%;
  min-width: 1150px;
  margin: 0 auto;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 30px;
}

.logo-group {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-shrink: 0;
}

.logo-img {
  height: 80px !important;
  width: auto;
  object-fit: contain;
}

.version-tag {
  color: #5eead4;
}

.nav-tabs :deep(.n-tabs-tab) {
  color: #64748b; /* 让未选中的稍微暗一点，突出选中的 */
  transition: color 0.3s ease;
}

.nav-tabs :deep(.n-tabs-tab:hover) {
  color: #5eead4; /* 悬停即亮 */
}

.header-right {
  display: flex;
  align-items: center;
  gap: 20px;
}

.guide-overlay {
  position: fixed;
  top: 0;
  left: 0;
  width: 100vw;
  height: 100vh;
  background: rgba(10, 25, 47, 0.9);
  backdrop-filter: blur(8px);
  z-index: 9999;
  display: flex;
  justify-content: center;
  align-items: center;
  cursor: pointer;
}

.guide-content {
  text-align: center;
  color: #e2e8f0;
}

.mouse-icon {
  width: 26px;
  height: 44px;
  border: 2px solid #5eead4;
  border-radius: 15px;
  margin: 0 auto 24px;
  position: relative;
}

.mouse-icon .wheel {
  width: 4px;
  height: 8px;
  background: #5eead4;
  border-radius: 2px;
  position: absolute;
  top: 8px;
  left: 50%;
  transform: translateX(-50%);
  animation: scroll-anim 1.6s infinite;
}

.status-glow {
  font-family: 'Courier New', Courier, monospace;
  font-size: 28px;
  font-weight: bold;
  color: #5eead4;
  letter-spacing: 2px;
  margin-bottom: 12px;
  animation: glow-pulse 2s infinite ease-in-out;
}

@keyframes glow-pulse {
  0%, 100% {
    opacity: 0.8;
    text-shadow: 0 0 5px rgba(94, 234, 212, 0.2);
  }
  50% {
    opacity: 1;
    text-shadow: 0 0 20px rgba(94, 234, 212, 0.6);
  }
}

.guide-sub-tip {
  font-size: 14px;
  color: #94a3b8;
  margin-bottom: 40px;
  letter-spacing: 1px;
}

.guide-btn {
  letter-spacing: 2px;
  padding: 0 25px;
}

@keyframes scroll-anim {
  0% {
    opacity: 1;
    transform: translate(-50%, 0);
  }
  100% {
    opacity: 0;
    transform: translate(-50%, 15px);
  }
}

.fade-enter-active, .fade-leave-active {
  transition: opacity 0.6s ease;
}

.fade-enter-from, .fade-leave-to {
  opacity: 0;
}
</style>