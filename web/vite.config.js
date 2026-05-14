import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue' // 注意这里多了一个 plugin-

export default defineConfig({
    plugins: [vue()],
    server: {
        proxy: {
            '/api': {
                target: 'http://localhost:3000',
                changeOrigin: true,
            }
        }
    }
})