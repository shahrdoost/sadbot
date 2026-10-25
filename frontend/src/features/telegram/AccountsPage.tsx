import { useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import {
  useTgCompleteLoginMutation,
  useTgDeleteAccountMutation,
  useTgListAccountsQuery,
  useTgStartLoginMutation,
  type TgAccount,
} from './telegramApi'
import { formatDateFa, toFaDigits } from '../../lib/utils'

type WizardStep = 'phone' | 'code' | 'password'

function AddAccountModal({ onClose }: { onClose: () => void }) {
  const [step, setStep] = useState<WizardStep>('phone')
  const [phone, setPhone] = useState('')
  const [label, setLabel] = useState('')
  const [code, setCode] = useState('')
  const [password, setPassword] = useState('')
  const [pendingId, setPendingId] = useState('')
  const [startLogin, startState] = useTgStartLoginMutation()
  const [completeLogin, completeState] = useTgCompleteLoginMutation()
  const [error, setError] = useState<string | null>(null)

  const errText = (e: unknown): string => {
    const data = (e as { data?: { error?: string } })?.data
    return data?.error || 'خطای ناشناخته'
  }

  const onSubmitPhone = async (e: FormEvent) => {
    e.preventDefault()
    setError(null)
    try {
      const res = await startLogin({ phone }).unwrap()
      setPendingId(res.pending_id)
      setStep('code')
    } catch (err) {
      setError(errText(err))
    }
  }

  const onSubmitCode = async (e: FormEvent) => {
    e.preventDefault()
    setError(null)
    try {
      await completeLogin({ pending_id: pendingId, phone, code, label }).unwrap()
      onClose()
    } catch (err) {
      const msg = errText(err)
      if (msg === 'PASSWORD_NEEDED') {
        setStep('password')
        return
      }
      setError(msg)
    }
  }

  const onSubmitPassword = async (e: FormEvent) => {
    e.preventDefault()
    setError(null)
    try {
      await completeLogin({ pending_id: pendingId, phone, code, password, label }).unwrap()
      onClose()
    } catch (err) {
      setError(errText(err))
    }
  }

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50 p-4">
      <div className="card w-full max-w-md p-6 space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="font-bold text-lg">افزودن اکانت تلگرام</h2>
          <button className="text-slate-400 dark:text-slate-500 hover:text-slate-600 dark:hover:text-slate-300" onClick={onClose} aria-label="بستن">
            ✕
          </button>
        </div>

        {error && <div className="text-sm text-red-600 dark:text-red-400 bg-red-50 dark:bg-red-950 border border-red-200 dark:border-red-800 rounded-lg p-3">{error}</div>}

        {step === 'phone' && (
          <form onSubmit={onSubmitPhone} className="space-y-3">
            <div>
              <label htmlFor="tg-phone" className="block text-sm font-medium mb-1">
                شماره تلفن (با کد کشور)
              </label>
              <input
                id="tg-phone"
                className="input"
                dir="ltr"
                placeholder="+98912xxxxxxx"
                value={phone}
                onChange={(e) => setPhone(e.target.value)}
                required
              />
            </div>
            <div>
              <label htmlFor="tg-label" className="block text-sm font-medium mb-1">
                برچسب (اختیاری)
              </label>
              <input id="tg-label" className="input" value={label} onChange={(e) => setLabel(e.target.value)} placeholder="مثلاً: اکانت اصلی" />
            </div>
            <button className="btn-primary w-full" disabled={startState.isLoading}>
              {startState.isLoading ? 'در حال ارسال…' : 'ارسال کد تایید'}
            </button>
          </form>
        )}

        {step === 'code' && (
          <form onSubmit={onSubmitCode} className="space-y-3">
            <p className="text-sm text-slate-600 dark:text-slate-300">
              کد تایید به شماره <span dir="ltr" className="font-mono">{toFaDigits(phone)}</span> ارسال شد.
            </p>
            <div>
              <label htmlFor="tg-code" className="block text-sm font-medium mb-1">
                کد تایید
              </label>
              <input
                id="tg-code"
                className="input"
                dir="ltr"
                inputMode="numeric"
                value={code}
                onChange={(e) => setCode(e.target.value)}
                required
              />
            </div>
            <button className="btn-primary w-full" disabled={completeState.isLoading}>
              {completeState.isLoading ? 'در حال ورود…' : 'ورود'}
            </button>
          </form>
        )}

        {step === 'password' && (
          <form onSubmit={onSubmitPassword} className="space-y-3">
            <p className="text-sm text-slate-600 dark:text-slate-300">این اکانت تایید دو مرحله‌ای دارد. رمز را وارد کنید.</p>
            <div>
              <label htmlFor="tg-pass" className="block text-sm font-medium mb-1">
                رمز تایید دو مرحله‌ای
              </label>
              <input
                id="tg-pass"
                className="input"
                type="password"
                dir="ltr"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
                autoFocus
              />
            </div>
            <button className="btn-primary w-full" disabled={completeState.isLoading}>
              {completeState.isLoading ? 'در حال ورود…' : 'تایید'}
            </button>
          </form>
        )}
      </div>
    </div>
  )
}

function AccountCard({ account, onDelete }: { account: TgAccount; onDelete: (id: string) => void }) {
  const initial = (account.display_name || account.label || account.phone || '؟').trim().charAt(0)
  return (
    <div className="card p-5 flex items-center gap-4">
      <div className="w-12 h-12 rounded-full bg-brand-600 text-white flex items-center justify-center text-xl font-bold shrink-0">
        {initial}
      </div>
      <div className="min-w-0 flex-1">
        <div className="font-bold truncate">{account.display_name || account.label || 'اکانت تلگرام'}</div>
        <div className="text-sm text-slate-500 dark:text-slate-400" dir="ltr">
          <span className="font-mono">{account.username ? `@${account.username}` : account.phone}</span>
        </div>
        <div className="text-xs text-slate-400 dark:text-slate-500 mt-1">افزوده شده: {formatDateFa(account.created_at)}</div>
      </div>
      <div className="flex flex-col gap-2 shrink-0">
        <Link to={`/telegram/${account.id}`} className="btn-primary text-sm px-4 py-1.5 text-center">
          ورود به چت‌ها
        </Link>
        <button
          className="btn-danger text-sm px-4 py-1.5"
          onClick={() => {
            if (confirm('این اکانت حذف شود؟ سشن تلگرام هم پاک می‌شود.')) onDelete(account.id)
          }}
        >
          حذف
        </button>
      </div>
    </div>
  )
}

export default function AccountsPage() {
  const { data, isLoading } = useTgListAccountsQuery()
  const [deleteAccount] = useTgDeleteAccountMutation()
  const [showAdd, setShowAdd] = useState(false)
  const accounts = data?.accounts ?? []

  return (
    <div className="p-6 space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-bold">📱 اکانت‌های تلگرام</h1>
          <p className="text-sm text-slate-500 dark:text-slate-400 mt-1">
            اکانت MTProto اضافه کنید و چت‌های آن را مثل تلگرام ببینید.
          </p>
        </div>
        <button className="btn-primary" onClick={() => setShowAdd(true)}>
          + افزودن اکانت
        </button>
      </div>

      {isLoading && <p className="text-slate-500 dark:text-slate-400 p-8 text-center">در حال بارگذاری…</p>}
      {!isLoading && accounts.length === 0 && (
        <div className="card p-12 text-center text-slate-400 dark:text-slate-500">
          هنوز اکانتی اضافه نکرده‌اید. با دکمهٔ «افزودن اکانت» شروع کنید.
        </div>
      )}

      <div className="grid gap-3">
        {accounts.map((a) => (
          <AccountCard key={a.id} account={a} onDelete={(id) => deleteAccount(id)} />
        ))}
      </div>

      {showAdd && <AddAccountModal onClose={() => setShowAdd(false)} />}
    </div>
  )
}
