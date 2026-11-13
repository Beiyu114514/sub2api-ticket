const KEY = 'ticket_theme'

export function applyTheme(theme) {
  document.documentElement.classList.toggle('dark', theme === 'dark')
  document.documentElement.style.colorScheme = theme === 'dark' ? 'dark' : 'light'
}

export function currentTheme() {
  return localStorage.getItem(KEY) || 'light'
}

export function setTheme(theme) {
  localStorage.setItem(KEY, theme)
  applyTheme(theme)
  return theme
}

export function toggleTheme() {
  return setTheme(currentTheme() === 'dark' ? 'light' : 'dark')
}
