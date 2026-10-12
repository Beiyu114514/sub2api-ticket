export function applyTheme(theme) {
  document.documentElement.classList.toggle('dark', theme === 'dark')
}

export function currentTheme() {
  return localStorage.getItem('ticket_theme') || 'light'
}
