// theme — light/dark persisted in localStorage under sadbot_theme.
// index.html applies the saved value before first paint; this module is the
// runtime API used by the sidebar toggle.

export type Theme = 'light' | 'dark'

const KEY = 'sadbot_theme'

export function getTheme(): Theme {
  try {
    return localStorage.getItem(KEY) === 'dark' ? 'dark' : 'light'
  } catch {
    return 'light'
  }
}

export function applyTheme(theme: Theme): void {
  const root = document.documentElement
  if (theme === 'dark') {
    root.classList.add('dark')
  } else {
    root.classList.remove('dark')
  }
  try {
    localStorage.setItem(KEY, theme)
  } catch {
    // private mode — theme just won't persist
  }
}

export function toggleTheme(): Theme {
  const next: Theme = getTheme() === 'dark' ? 'light' : 'dark'
  applyTheme(next)
  return next
}
