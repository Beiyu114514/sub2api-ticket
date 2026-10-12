import { createRouter, createWebHistory } from 'vue-router'
import TicketList from './views/TicketList.vue'
import TicketNew from './views/TicketNew.vue'
import TicketDetail from './views/TicketDetail.vue'
import Settings from './views/Settings.vue'
import AdminSettings from './views/AdminSettings.vue'
import Login from './views/Login.vue'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', component: TicketList },
    { path: '/new', component: TicketNew },
    { path: '/tickets/:id', component: TicketDetail },
    { path: '/settings', component: Settings },
    { path: '/admin', component: AdminSettings, meta: { admin: true } },
    { path: '/login', component: Login },
  ],
})
