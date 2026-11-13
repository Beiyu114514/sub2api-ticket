import { reactive } from 'vue'

export const store = reactive({
  user: null,
  booted: false,
  bootError: '',
  sidebarOpen: false,
  get isAdmin() { return this.user?.role === 'admin' },
})
