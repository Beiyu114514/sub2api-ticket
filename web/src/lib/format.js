/** 相对/绝对时间格式化，统一走这里，避免各页面各写一套 */
export function fmtDateTime(iso) {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  return d.toLocaleString([], {
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit',
  })
}

/** 列表用的紧凑时间：今天只显示时分 */
export function fmtShortTime(iso) {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  const now = new Date()
  const sameDay = d.toDateString() === now.toDateString()
  if (sameDay) return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  const sameYear = d.getFullYear() === now.getFullYear()
  return d.toLocaleDateString([], {
    ...(sameYear ? {} : { year: 'numeric' }),
    month: '2-digit', day: '2-digit',
  }) + ' ' + d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

/** 相对时间，用于“最近活动” */
export function fmtRelative(iso) {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  const diff = Date.now() - d.getTime()
  const min = Math.floor(diff / 60000)
  if (min < 1) return '<1m'
  if (min < 60) return `${min}m`
  const hrs = Math.floor(min / 60)
  if (hrs < 24) return `${hrs}h`
  const days = Math.floor(hrs / 24)
  if (days < 30) return `${days}d`
  return fmtShortTime(iso)
}

/** 安全解析消息附件 JSON */
export function parseAttachments(raw) {
  if (!raw) return []
  if (Array.isArray(raw)) return raw
  try {
    const v = JSON.parse(raw)
    return Array.isArray(v) ? v : []
  } catch {
    return []
  }
}

/** 取用户名首字母作为头像 fallback */
export function initials(name) {
  const s = String(name || '').trim()
  if (!s) return '?'
  return s.slice(0, 1).toUpperCase()
}
