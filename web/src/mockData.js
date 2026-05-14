// Tatai V2.0 模拟数据[cite: 2]
export const initialApps = [
  { 
    id: 1, name: 'Tatai-Server', type: 'java', status: 'UP', 
    check_cmd: 'pgrep -f tatai.jar', last_exit_code: 0 
  },
  { 
    id: 2, name: 'MySQL-Container', type: 'docker', status: 'UP', 
    check_cmd: 'docker ps -q -f name=mysql', last_exit_code: 0 
  },
  { 
    id: 3, name: 'Nginx-Gateway', type: 'shell', status: 'DOWN', 
    check_cmd: 'pgrep nginx', last_exit_code: 1 
  }
]

export const sysMetricsData = [
  { label: 'CPU 负载', val: 28, color: '#2dd4bf' },
  { label: '内存使用', val: 64, color: '#3b82f6' },
  { label: '磁盘健康', val: 9, color: '#10b981' }
]