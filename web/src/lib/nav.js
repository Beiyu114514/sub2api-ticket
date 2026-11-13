import { computed } from 'vue'
import { store } from '@/store'
import { t } from '@/i18n'
import {
  Ticket,
  Plus,
  Settings,
  SlidersHorizontal,
} from '@lucide/vue'

/**
 * 侧边栏导航配置。
 * 与 shadcn 的 sidebar-07 block 一致：分组 + 图标 + 当前项高亮，
 * 管理员额外多一个「系统设置」分组。
 */
export function useNavGroups() {
  return computed(() => {
    const groups = [
      {
        label: t('workspace'),
        items: [
          { to: '/', label: store.isAdmin ? t('navAllTickets') : t('navTickets'), icon: Ticket },
          { to: '/new', label: t('navNew'), icon: Plus },
        ],
      },
      {
        label: t('account'),
        items: [
          { to: '/settings', label: t('navSettings'), icon: Settings },
        ],
      },
    ]
    if (store.isAdmin) {
      groups.push({
        label: t('navManage'),
        items: [{ to: '/admin', label: t('navAdmin'), icon: SlidersHorizontal }],
      })
    }
    return groups
  })
}
