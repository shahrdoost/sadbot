import { afterEach, describe, expect, it } from 'vitest'
import { applyTheme, getTheme, toggleTheme } from './theme'

afterEach(() => {
  localStorage.clear()
  document.documentElement.classList.remove('dark')
})

describe('theme', () => {
  it('defaults to light when nothing is stored', () => {
    expect(getTheme()).toBe('light')
    expect(document.documentElement.classList.contains('dark')).toBe(false)
  })

  it('applyTheme(dark) toggles the html class and persists', () => {
    applyTheme('dark')
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(localStorage.getItem('sadbot_theme')).toBe('dark')
    expect(getTheme()).toBe('dark')
  })

  it('toggleTheme flips and persists', () => {
    expect(toggleTheme()).toBe('dark')
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(toggleTheme()).toBe('light')
    expect(document.documentElement.classList.contains('dark')).toBe(false)
    expect(localStorage.getItem('sadbot_theme')).toBe('light')
  })

  it('applyTheme(light) removes the class', () => {
    applyTheme('dark')
    applyTheme('light')
    expect(document.documentElement.classList.contains('dark')).toBe(false)
  })
})
