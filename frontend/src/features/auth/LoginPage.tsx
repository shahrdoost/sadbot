import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useLoginMutation } from '../bots/botsApi'

export default function LoginPage() {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [login, { isLoading, error }] = useLoginMutation()
  const navigate = useNavigate()

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault()
    try {
      const res = await login({ email, password }).unwrap()
      localStorage.setItem('sadbot_token', res.access_token)
      localStorage.setItem('sadbot_admin', JSON.stringify(res.admin))
      navigate('/', { replace: true })
    } catch {
      /* error rendered below */
    }
  }

  const errMsg =
    error && typeof error === 'object' && 'data' in error
      ? ((error.data as { error?: string })?.error ?? 'خطا در ورود')
      : null

  return (
    <div className="min-h-screen flex items-center justify-center bg-gradient-to-bl from-brand-50 to-slate-100 dark:to-slate-800 p-4">
      <div className="card w-full max-w-md p-8">
        <div className="text-center mb-8">
          <h1 className="text-3xl font-bold text-slate-800 dark:text-slate-100">صدبات</h1>
          <p className="text-slate-500 dark:text-slate-400 mt-2">پنل مرکزی مدیریت بات‌های تلگرام</p>
        </div>
        <form onSubmit={onSubmit} className="space-y-4">
          <div>
            <label htmlFor="login-email" className="block text-sm font-medium mb-1">ایمیل</label>
            <input
              id="login-email"
              className="input"
              type="email"
              dir="ltr"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              required
              autoComplete="email"
            />
          </div>
          <div>
            <label htmlFor="login-password" className="block text-sm font-medium mb-1">رمز عبور</label>
            <input
              id="login-password"
              className="input"
              type="password"
              dir="ltr"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
              autoComplete="current-password"
            />
          </div>
          {errMsg && (
            <div className="text-sm text-red-600 dark:text-red-400 bg-red-50 dark:bg-red-950 border border-red-200 dark:border-red-800 rounded-lg p-3">
              {errMsg}
            </div>
          )}
          <button className="btn-primary w-full" disabled={isLoading}>
            {isLoading ? 'در حال ورود…' : 'ورود'}
          </button>
        </form>
      </div>
    </div>
  )
}
