import { reactive } from 'vue'

export const store = reactive({
  user: null,
  booted: false,
  bootError: '',
  get isAdmin() { return this.user?.role === 'admin' },
})
