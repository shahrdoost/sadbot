import { useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import {
  useCreateBotFromAccountMutation,
  useCreateBotMutation,
  useListBotsQuery,
  type Bot,
} from './botsApi'
import { useTgListAccountsQuery } from '../telegram/telegramApi'
import { formatDateFa } from '../../lib/utils'

const templateLabels: Record<string, string> = {
  welcome_menu: 'منوی خوش‌آمد (نمونه)',
  dating: 'دوستیابی (kpkchat)',
  content: 'محتوا (اخبار کانال)',
  bombland: 'کارخانه بومبلند (بازی)',
  duel: 'سنگ‌کاغذ‌قیچی (بازی)',
  favaani: 'بازی فراوانی (ذهنیت پول)',
  shlinks: 'لینک‌ساز (کوتاه‌کننده)',
  whoseeme: 'کی منو دیده (لینک طعمه)',
  faal: 'فال روزانه',
  anonychat: 'چت ناشناس',
  tapearn: 'تپ‌کلیک (کسب درآمد)',
}

function StatusBadge({ bot }: { bot: Bot }) {
  if (bot.running) {
    return <span className="inline-flex items-center gap-1 text-xs bg-green-100 dark:bg-green-900 text-green-700 dark:text-green-400 rounded-full px-2 py-0.5">● در حال اجرا</span>
  }
  const map: Record<string, string> = {
    stopped: 'bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-300',
    error: 'bg-red-100 dark:bg-red-900 text-red-700 dark:text-red-300',
    starting: 'bg-amber-100 dark:bg-amber-900 text-amber-700 dark:text-amber-400',
    running: 'bg-green-100 dark:bg-green-900 text-green-700 dark:text-green-400',
  }
  return (
    <span className={`text-xs rounded-full px-2 py-0.5 ${map[bot.status] ?? map.stopped}`}>
      {bot.status === 'stopped' ? 'متوقف' : bot.status === 'error' ? 'خطا' : bot.status === 'starting' ? 'در حال راه‌اندازی' : 'در حال اجرا'}
    </span>
  )
}

export default function BotsPage() {
  const { data, isLoading } = useListBotsQuery()
  const [createBot, { isLoading: creating, error: createError }] = useCreateBotMutation()
  const [createFromAccount, { isLoading: creatingFromAccount, error: createFromAccountError }] =
    useCreateBotFromAccountMutation()
  const { data: accountsData } = useTgListAccountsQuery()
  const accounts = accountsData?.accounts ?? []

  const [showCreate, setShowCreate] = useState(false)
  const [mode, setMode] = useState<'account' | 'token'>('account')
  const [name, setName] = useState('')
  const [token, setToken] = useState('')
  const [templateKey, setTemplateKey] = useState('welcome_menu')
  const [accountId, setAccountId] = useState('')
  const [usernameHint, setUsernameHint] = useState('')

  const resetForm = () => {
    setName('')
    setToken('')
    setUsernameHint('')
    setAccountId('')
  }

  const errOf = (e: unknown): string | null =>
    e && typeof e === 'object' && 'data' in e
      ? ((e.data as { error?: string })?.error ?? 'خطا در ساخت بات')
      : null

  const createErrMsg = errOf(createError)
  const createFromAccountErrMsg = errOf(createFromAccountError)

  const onCreate = async (e: FormEvent) => {
    e.preventDefault()
    try {
      if (mode === 'account') {
        await createFromAccount({
          account_id: accountId || accounts[0]?.id || '',
          name,
          template_key: templateKey,
          username_hint: usernameHint || undefined,
        }).unwrap()
      } else {
        await createBot({ name, token, template_key: templateKey }).unwrap()
      }
      setShowCreate(false)
      resetForm()
    } catch {
      /* shown below */
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-2xl font-bold">بات‌ها</h2>
        <button className="btn-primary" onClick={() => setShowCreate(true)}>
          + ساخت بات جدید
        </button>
      </div>

      {showCreate && (
        <div className="card p-6">
          <div className="flex items-center justify-between mb-4">
            <h3 className="font-semibold">ساخت بات جدید</h3>
            <div className="flex gap-1 bg-slate-100 dark:bg-slate-800 rounded-lg p-1">
              <button
                type="button"
                className={`px-3 py-1.5 rounded-md text-sm ${mode === 'account' ? 'bg-white dark:bg-slate-900 shadow text-brand-700 dark:text-brand-500 font-medium' : 'text-slate-600 dark:text-slate-300'}`}
                onClick={() => setMode('account')}
              >
                از اکانت تلگرام
              </button>
              <button
                type="button"
                className={`px-3 py-1.5 rounded-md text-sm ${mode === 'token' ? 'bg-white dark:bg-slate-900 shadow text-brand-700 dark:text-brand-500 font-medium' : 'text-slate-600 dark:text-slate-300'}`}
                onClick={() => setMode('token')}
              >
                با توکن
              </button>
            </div>
          </div>
          <form onSubmit={onCreate} className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {mode === 'account' && (
              <div className="md:col-span-2">
                <label htmlFor="bot-account" className="block text-sm font-medium mb-1">
                  ساخت بات از طریق کدام اکانت؟
                </label>
                <select
                  id="bot-account"
                  className="input"
                  value={accountId || accounts[0]?.id || ''}
                  onChange={(e) => setAccountId(e.target.value)}
                  required
                >
                  {accounts.length === 0 && <option value="">اکانتی اضافه نکرده‌اید</option>}
                  {accounts.map((a) => (
                    <option key={a.id} value={a.id}>
                      {a.display_name || a.label || a.phone}
                      {a.username ? ` (@${a.username})` : ''}
                    </option>
                  ))}
                </select>
                <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
                  ساخت با @BotFather از این اکانت انجام می‌شود؛ ۳۰ تا ۶۰ ثانیه طول می‌کشد.
                </p>
              </div>
            )}
            <div>
              <label htmlFor="bot-name" className="block text-sm font-medium mb-1">نام بات</label>
              <input id="bot-name" className="input" value={name} onChange={(e) => setName(e.target.value)} required />
            </div>
            <div>
              <label htmlFor="bot-template" className="block text-sm font-medium mb-1">تمپلیت</label>
              <select id="bot-template" className="input" value={templateKey} onChange={(e) => setTemplateKey(e.target.value)}>
                <option value="welcome_menu">{templateLabels.welcome_menu}</option>
                <option value="dating">{templateLabels.dating}</option>
                <option value="content">{templateLabels.content}</option>
                <option value="bombland">{templateLabels.bombland}</option>
                <option value="duel">{templateLabels.duel}</option>
                <option value="favaani">{templateLabels.favaani}</option>
                <option value="shlinks">{templateLabels.shlinks}</option>
                <option value="whoseeme">{templateLabels.whoseeme}</option>
                <option value="faal">{templateLabels.faal}</option>
                <option value="anonychat">{templateLabels.anonychat}</option>
                <option value="tapearn">{templateLabels.tapearn}</option>
              </select>
            </div>
            {mode === 'account' && (
              <div className="md:col-span-2">
                <label htmlFor="bot-hint" className="block text-sm font-medium mb-1">
                  پایهٔ آیدی (اختیاری، لاتین)
                </label>
                <input
                  id="bot-hint"
                  className="input font-mono"
                  dir="ltr"
                  placeholder="MyCoolBot"
                  value={usernameHint}
                  onChange={(e) => setUsernameHint(e.target.value)}
                />
                <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
                  اگر خالی بماند، از روی نام بات آیدی ساخته می‌شود تا یکی آزاد پیدا شود.
                </p>
              </div>
            )}
            {mode === 'token' && (
              <div className="md:col-span-2">
                <label htmlFor="bot-token" className="block text-sm font-medium mb-1">توکن ربات تلگرام</label>
                <input
                  id="bot-token"
                  className="input font-mono"
                  dir="ltr"
                  placeholder="123456789:AA..."
                  value={token}
                  onChange={(e) => setToken(e.target.value)}
                  required
                />
                <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
                  توکن از @BotFather — قبل از ذخیره با تلگرام اعتبارسنجی می‌شود.
                </p>
              </div>
            )}
            {mode === 'account' && accounts.length === 0 && (
              <div className="md:col-span-2 text-sm text-amber-700 dark:text-amber-400 bg-amber-50 dark:bg-amber-950 border border-amber-200 dark:border-amber-800 rounded-lg p-3">
                اول باید از بخش «اکانت‌های تلگرام» یک اکانت اضافه کنید، یا همین‌جا با توکن بسازید.
              </div>
            )}
            {(createErrMsg || createFromAccountErrMsg) && (
              <div className="md:col-span-2 text-sm text-red-600 dark:text-red-400 bg-red-50 dark:bg-red-950 border border-red-200 dark:border-red-800 rounded-lg p-3">
                {mode === 'account' ? createFromAccountErrMsg : createErrMsg}
              </div>
            )}
            <div className="md:col-span-2 flex gap-2">
              <button
                className="btn-primary"
                disabled={creating || creatingFromAccount || (mode === 'account' && accounts.length === 0)}
              >
                {creatingFromAccount
                  ? 'در حال گفتگو با BotFather…'
                  : creating
                    ? 'در حال ساخت…'
                    : 'ساخت بات'}
              </button>
              <button type="button" className="btn-secondary" onClick={() => setShowCreate(false)}>
                انصراف
              </button>
            </div>
          </form>
        </div>
      )}

      <div className="card overflow-hidden">
        {isLoading ? (
          <div className="p-8 text-center text-slate-400 dark:text-slate-500">در حال بارگذاری…</div>
        ) : !data?.bots.length ? (
          <div className="p-8 text-center text-slate-400 dark:text-slate-500">هنوز باتی ندارید. اولین بات را بسازید.</div>
        ) : (
          <table className="w-full text-sm">
            <thead className="bg-slate-50 dark:bg-slate-900 text-slate-500 dark:text-slate-400">
              <tr>
                <th className="text-start px-5 py-3 font-medium">نام</th>
                <th className="text-start px-5 py-3 font-medium">کاربر تلگرام</th>
                <th className="text-start px-5 py-3 font-medium">تمپلیت</th>
                <th className="text-start px-5 py-3 font-medium">وضعیت</th>
                <th className="text-start px-5 py-3 font-medium">تاریخ ساخت</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
              {data.bots.map((bot) => (
                <tr key={bot.id} className="hover:bg-slate-50 dark:hover:bg-slate-800">
                  <td className="px-5 py-3">
                    <Link to={`/bots/${bot.id}`} className="text-brand-600 dark:text-brand-500 hover:underline font-medium">
                      {bot.name}
                    </Link>
                  </td>
                  <td className="px-5 py-3 font-mono text-xs" dir="ltr">
                    {bot.telegram_username ?? '—'}
                  </td>
                  <td className="px-5 py-3">{templateLabels[bot.template_key] ?? bot.template_key}</td>
                  <td className="px-5 py-3">
                    <StatusBadge bot={bot} />
                  </td>
                  <td className="px-5 py-3 text-slate-500 dark:text-slate-400">{formatDateFa(bot.created_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}
