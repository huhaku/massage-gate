import axios from 'axios'
import router from './router'
import { ElMessage } from 'element-plus'

const http = axios.create({ timeout: 30000 })

http.interceptors.response.use(
  (r) => r,
  (err) => {
    const status = err.response?.status
    const msg = err.response?.data?.error || err.message
    if (status === 401 && router.currentRoute.value.path !== '/login') {
      router.push('/login')
    }
    return Promise.reject(new Error(msg))
  }
)

export function showError(e) {
  ElMessage.error(e?.message || String(e))
}

export const api = {
  get: (url, params) => http.get(url, { params }).then((r) => r.data),
  post: (url, data) => http.post(url, data).then((r) => r.data),
  put: (url, data) => http.put(url, data).then((r) => r.data),
  del: (url) => http.delete(url).then((r) => r.data),
}

export function fmtTime(iso) {
  if (!iso) return '-'
  return new Date(iso).toLocaleString('zh-CN', { hour12: false })
}

export const STATUS = {
  success: { label: '已送达', type: 'success' },
  queued: { label: '暂存重试中', type: 'warning' },
  pending: { label: '待投递', type: 'primary' },
  sending: { label: '投递中', type: 'primary' },
  dead: { label: '无可用目标', type: 'danger' },
  unrouted: { label: '未命中路由', type: 'info' },
}

export const SOURCE_TYPES = {
  gotify: 'Gotify',
  ntfy: 'ntfy',
  bark: 'Bark',
  webhook: '通用 Webhook',
  telegram: 'Telegram Bot',
  wecom: '企业微信',
  dingtalk: '钉钉机器人',
}

export const TARGET_TYPES = {
  gotify: 'Gotify',
  ntfy: 'ntfy',
  bark: 'Bark',
  telegram: 'Telegram Bot',
  wecom: '企业微信群机器人',
  feishu: '飞书自定义机器人',
  dingtalk: '钉钉自定义机器人',
  webhook: '通用 Webhook',
  custom: '自定义 HTTP 模板',
}
