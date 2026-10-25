import { useState, type FormEvent } from 'react'
import { useCreateAdminMutation, useDeleteAdminMutation, useListAdminsQuery, type Admin } from '../bots/botsApi'
import { formatDateFa } from '../../lib/utils'

export default function AdminsPage() {
  const { data, isLoading } = useListAdminsQuery()
  const [createAdmin, { isLoading: creating, error }] = useCreateAdminMutation()
  const [deleteAdmin] = useDeleteAdminMutation()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [role, setRole] = useState('admin')

  const onCreate = async (e: FormEvent) => {
    e.preventDefault()
    await createAdmin({ email, password, display_name: displayName, role }).unwrap()
    setEmail('')
    setPassword('')
    setDisplayName('')
    setRole('admin')
  }

  const errMsg = error && typeof error === 'object' && 'data' in error
    ? ((error.data as { error?: string })?.error ?? 'خطا در ساخت مدیر')
    : null

  const onDelete = async (a: Admin) => {
    if (!confirm(`مدیر «${a.display_name}» حذف شود؟`)) return
    await deleteAdmin(a.id)
  }

  return (
    <div className="space-y-6">
      <h2 className="text-2xl font-bold">مدیران</h2>

      <form onSubmit={onCreate} className="card p-5 grid grid-cols-1 md:grid-cols-5 gap-3 items-end">
        <div>
          <label className="block text-xs text-slate-500 dark:text-slate-400 mb-1">نام نمایشی</label>
          <input className="input" value={displayName} onChange={(e) => setDisplayName(e.target.value)} required />
        </div>
        <div>
          <label className="block text-xs text-slate-500 dark:text-slate-400 mb-1">ایمیل</label>
          <input className="input" dir="ltr" type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
        </div>
        <div>
          <label className="block text-xs text-slate-500 dark:text-slate-400 mb-1">رمز عبور</label>
          <input className="input" dir="ltr" type="password" value={password} onChange={(e) => setPassword(e.target.value)} required minLength={8} />
        </div>
        <div>
          <label className="block text-xs text-slate-500 dark:text-slate-400 mb-1">نقش</label>
          <select className="input" value={role} onChange={(e) => setRole(e.target.value)}>
            <option value="admin">مدیر</option>
            <option value="superadmin">مدیر اصلی</option>
          </select>
        </div>
        <button className="btn-primary" disabled={creating}>{creating ? 'در حال ساخت…' : 'ساخت مدیر'}</button>
        {errMsg && <div className="md:col-span-5 text-sm text-red-600 dark:text-red-400">{errMsg}</div>}
      </form>

      <div className="card overflow-hidden">
        {isLoading ? (
          <div className="p-8 text-center text-slate-400 dark:text-slate-500">در حال بارگذاری…</div>
        ) : (
          <table className="w-full text-sm">
            <thead className="bg-slate-50 dark:bg-slate-900 text-slate-500 dark:text-slate-400">
              <tr>
                <th className="text-start px-5 py-3 font-medium">نام</th>
                <th className="text-start px-5 py-3 font-medium">ایمیل</th>
                <th className="text-start px-5 py-3 font-medium">نقش</th>
                <th className="text-start px-5 py-3 font-medium">وضعیت</th>
                <th className="text-start px-5 py-3 font-medium">تاریخ ساخت</th>
                <th className="px-5 py-3"></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
              {data?.admins.map((a) => (
                <tr key={a.id} className="hover:bg-slate-50 dark:hover:bg-slate-800">
                  <td className="px-5 py-3 font-medium">{a.display_name}</td>
                  <td className="px-5 py-3 font-mono text-xs" dir="ltr">{a.email}</td>
                  <td className="px-5 py-3">{a.role === 'superadmin' ? 'مدیر اصلی' : 'مدیر'}</td>
                  <td className="px-5 py-3">
                    <span className={`text-xs rounded-full px-2 py-0.5 ${a.status === 'active' ? 'bg-green-100 dark:bg-green-900 text-green-700 dark:text-green-400' : 'bg-slate-100 dark:bg-slate-800 text-slate-500 dark:text-slate-400'}`}>
                      {a.status === 'active' ? 'فعال' : 'غیرفعال'}
                    </span>
                  </td>
                  <td className="px-5 py-3 text-slate-500 dark:text-slate-400">{formatDateFa(a.created_at)}</td>
                  <td className="px-5 py-3 text-end">
                    <button className="text-red-600 dark:text-red-400 hover:text-red-800 dark:hover:text-red-300 text-xs" onClick={() => onDelete(a)}>
                      حذف
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <p className="text-xs text-slate-500 dark:text-slate-400">
        برای دادن دسترسی هر مدیر به هر بات، از API استفاده کنید:
        <code className="mx-1" dir="ltr">PUT /api/v1/admins/&#123;adminId&#125;/bot-access/&#123;botId&#125;</code>
        با بدنهٔ <code dir="ltr">{'{"permissions":["bot:read","bot:users"]}'}</code>
      </p>
    </div>
  )
}
