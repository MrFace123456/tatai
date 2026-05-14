import { createApp } from 'vue'
import App from './App.vue'
import axios from 'axios'

async function initApp() {
    if (import.meta.env.DEV) {
        // 开发环境：使用 Vite 代理
        axios.defaults.baseURL = '/api'
        console.log('[Dev] API base URL:', axios.defaults.baseURL)
    } else {
        // 生产环境：加载 config.json
        try {
            const response = await fetch('/config.json')
            const config = await response.json()
            axios.defaults.baseURL = config.apiBaseURL || '/api'
            console.log('[Prod] API base URL:', axios.defaults.baseURL)
        } catch (error) {
            console.error('加载 config.json 失败，使用默认 /api', error)
            axios.defaults.baseURL = '/api'
        }
    }

    const app = createApp(App)
    app.mount('#app')
}

initApp()