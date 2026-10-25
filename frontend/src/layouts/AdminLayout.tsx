import { useState } from 'react'
import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import { getTheme, toggleTheme, type Theme } from '../lib/theme'

const navItems = [
  { to: '/', label: '📊 داشبورد و مالی', end: true },
  { to: '/bots', label: '🤖 بات‌ها' },
  { to: '/telegram', label: '📱 اکانت‌های تلگرام' },
  { to: '/admins', label: '👤 مدیران' },
]

export default function AdminLayout() {
  const navigate = useNavigate()
  const [theme, setThemeState] = useState<Theme>(() => getTheme())
  const adminRaw = localStorage.getItem('sadbot_admin')
  const admin = adminRaw ? (JSON.parse(adminRaw) as { display_name: string; role: string }) : null

  const logout = () => {
    localStorage.removeItem('sadbot_token')
    localStorage.removeItem('sadbot_admin')
    navigate('/login', { replace: true })
  }

  const onToggleTheme = () => setThemeState(toggleTheme())

  return (
    <div className="min-h-screen flex">
      <aside className="w-64 bg-slate-900 text-slate-100 flex flex-col shrink-0">
        <div className="p-5 border-b border-slate-800 flex items-center justify-between">
          <div>
            <h1 className="text-xl font-bold">صدبات</h1>
            <p className="text-xs text-slate-400 mt-1">مدیریت بات‌های تلگرام</p>
          </div>
          <button
            onClick={onToggleTheme}
            title={theme === 'dark' ? 'حالت روشن' : 'حالت تاریک'}
            aria-label={theme === 'dark' ? 'فعال‌کردن حالت روشن' : 'فعال‌کردن حالت تاریک'}
            className="rounded-lg p-2 text-slate-300 hover:bg-slate-800 hover:text-white transition-colors"
          >
            {theme === 'dark' ? '☀️' : '🌙'}
          </button>
        </div>
        <nav className="flex-1 p-3 space-y-1">
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) =>
                `block rounded-lg px-3 py-2 text-sm transition-colors ${
                  isActive ? 'bg-brand-600 text-white' : 'text-slate-300 hover:bg-slate-800'
                }`
              }
            >
              {item.label}
            </NavLink>
          ))}
        </nav>
        <div className="p-4 border-t border-slate-800">
          {admin && (
            <div className="text-xs text-slate-400 mb-2">
              <div className="font-medium text-slate-200">{admin.display_name}</div>
              <div>{admin.role === 'superadmin' ? 'مدیر اصلی' : 'مدیر'}</div>
            </div>
          )}
          <button onClick={logout} className="text-sm text-red-400 hover:text-red-300">
            خروج
          </button>
        </div>
      </aside>
      <main className="flex-1 p-6 overflow-x-auto">
        <Outlet />
      </main>
    </div>
  )
}
