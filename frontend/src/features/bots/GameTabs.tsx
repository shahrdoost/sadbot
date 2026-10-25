import { useState } from 'react'
import {
  useGetSettingsQuery,
  usePutSettingsMutation,
} from './botsApi'

interface GameSettingsTabProps {
  botId: string
  templateKey: string
}

export function GameSettingsTab({ botId, templateKey }: GameSettingsTabProps) {
  const { data: settings, isLoading } = useGetSettingsQuery(botId)
  const [putSettings, { isLoading: saving }] = usePutSettingsMutation()
  const [values, setValues] = useState<Record<string, number>>({})
  const [msg, setMsg] = useState('')

  if (isLoading || !settings) return <div className="text-slate-500 p-4">در حال بارگذاری…</div>

  const fields = 'fields' in settings ? settings.fields : (settings as any)
  const groups: Record<string, any[]> = {}
  for (const f of (Array.isArray(fields) ? fields : [])) {
    if (!groups[f.group]) groups[f.group] = []
    groups[f.group].push(f)
  }

  const groupLabels: Record<string, string> = {
    economy: '💰 اقتصاد',
    energy: '⚡ انرژی',
    drops: '🎁 جوایز تصادفی',
    prizes: '💎 جوایز واقعی',
    growth: '📈 رشد',
  }

  const handleSave = async () => {
    setMsg('')
    try {
      await putSettings({ botId, version: 0, values }).unwrap()
      setMsg('✅ تنظیمات ذخیره شد.')
    } catch (e: any) {
      setMsg('❌ خطا: ' + (e?.data?.error || e?.message || 'نامشخص'))
    }
  }

  return (
    <div className="p-4 space-y-6">
      <h2 className="text-lg font-bold text-slate-800 dark:text-slate-200">
        🎮 تنظیمات بازی {templateKey === 'bombland' ? 'کارخانه بومبلند' : 'سنگ‌کاغذ‌قیچی'}
      </h2>

      {Object.entries(groups).map(([group, fields]) => (
        <div key={group} className="bg-white dark:bg-slate-800 rounded-xl border border-slate-200 dark:border-slate-700 p-4">
          <h3 className="font-bold text-slate-700 dark:text-slate-300 mb-3">
            {groupLabels[group] || group}
          </h3>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {fields.map((f) => (
              <label key={f.key} className="block">
                <span className="text-sm text-slate-600 dark:text-slate-400">{f.label}</span>
                <input
                  type="number"
                  min={f.min}
                  max={f.max}
                  value={values[f.key] ?? f.value}
                  onChange={(e) => setValues({ ...values, [f.key]: Number(e.target.value) })}
                  className="mt-1 w-full rounded-lg border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-900 px-3 py-2 text-sm"
                />
                <span className="text-xs text-slate-400">بازه: {f.min} تا {f.max}</span>
              </label>
            ))}
          </div>
        </div>
      ))}

      <div className="flex items-center gap-3">
        <button
          onClick={handleSave}
          disabled={saving}
          className="px-6 py-2 bg-brand-600 text-white rounded-lg hover:bg-brand-700 disabled:opacity-50"
        >
          {saving ? 'در حال ذخیره…' : '💾 ذخیره تنظیمات'}
        </button>
        {msg && <span className="text-sm">{msg}</span>}
      </div>
    </div>
  )
}

interface GamePlayersTabProps {
  templateKey: string
}

export function GamePlayersTab({ templateKey }: GamePlayersTabProps) {
  // For now show a simple message — player list API can be added later
  return (
    <div className="p-4">
      <div className="bg-white dark:bg-slate-800 rounded-xl border border-slate-200 dark:border-slate-700 p-8 text-center">
        <div className="text-4xl mb-4">{templateKey === 'bombland' ? '🏭' : '🎮'}</div>
        <h3 className="text-lg font-bold text-slate-700 dark:text-slate-300 mb-2">
          {templateKey === 'bombland' ? 'بازیکنان کارخانه بومبلند' : 'بازیکنان سنگ‌کاغذ‌قیچی'}
        </h3>
        <p className="text-slate-500 dark:text-slate-400">
          لیست بازیکنان به‌زودی در این بخش نمایش داده می‌شود.
        </p>
      </div>
    </div>
  )
}

export function WithdrawalsTab(_props: { botId: string }) {
  return (
    <div className="p-4">
      <div className="bg-white dark:bg-slate-800 rounded-xl border border-slate-200 dark:border-slate-700 p-8 text-center">
        <div className="text-4xl mb-4">💎</div>
        <h3 className="text-lg font-bold text-slate-700 dark:text-slate-300 mb-2">درخواست‌های برداشت</h3>
        <p className="text-slate-500 dark:text-slate-400">
          مدیریت درخواست‌های برداشت جوایز به‌زودی فعال می‌شود.
        </p>
      </div>
    </div>
  )
}
