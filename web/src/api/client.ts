import axios from 'axios'

const client = axios.create({
  baseURL: '/api',
  timeout: 15000,
})

client.interceptors.response.use(
  (r) => r,
  (err) => {
    const status = err.response?.status
    const url: string = err.config?.url ?? ''
    if (status === 401 && !url.includes('/login') && !url.includes('/auth/status')) {
      const next = encodeURIComponent(window.location.pathname + window.location.search)
      window.location.href = `/login?next=${next}`
    }
    const msg = err.response?.data?.error?.message ?? '请求失败，请稍后再试'
    return Promise.reject(new Error(msg))
  },
)

export default client
