async function req(method, url, body) {
  const res = await fetch(url, {
    method,
    headers: body !== undefined ? { 'Content-Type': 'application/json' } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
    credentials: 'same-origin',
  })
  let data = {}
  try { data = await res.json() } catch { /* 空响应 */ }
  if (!res.ok) {
    const err = new Error(data.error || `HTTP ${res.status}`)
    err.status = res.status
    throw err
  }
  return data
}

export const api = {
  get: (url) => req('GET', url),
  post: (url, body) => req('POST', url, body ?? {}),
  put: (url, body) => req('PUT', url, body ?? {}),
  del: (url) => req('DELETE', url),
}
