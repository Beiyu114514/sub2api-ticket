import { createApp } from 'vue'
import App from './App.vue'
import { router } from './router'
import { api } from './api'
import { store } from './store'
import { applyTheme } from './theme'
import './i18n'
import './style.css'

// —— iframe 入口参数处理：theme / lang / SSO token ——
const q = new URLSearchParams(location.search)
const theme = q.get('theme')
const lang = q.get('lang')
if (theme === 'light' || theme === 'dark') {
  localStorage.setItem('ticket_theme', theme)
} else if (!localStorage.getItem('ticket_theme')) {
  localStorage.setItem('ticket_theme', 'light')
}
if (lang === 'en' || lang === 'zh') localStorage.setItem('ticket_lang', lang)
applyTheme(localStorage.getItem('ticket_theme'))
window.__appLang = localStorage.getItem('ticket_lang') || 'zh'

async function boot() {
  const token = q.get('token')
  const userId = Number(q.get('user_id') || 0)
  try {
    if (token) {
      // SSO：透传 JWT 换自有 HttpOnly cookie，然后立刻抹掉 URL 上的 token
      await api.post('/api/v1/auth/sso', { token, user_id: userId })
      history.replaceState(null, '', location.pathname)
    }
    const me = await api.get('/api/v1/me')
    store.user = me.user
  } catch (e) {
    store.bootError = e.status === 401 ? 'unauthorized' : 'network'
  }
  store.booted = true
}

boot().then(() => {
  createApp(App).use(router).mount('#app')
})
