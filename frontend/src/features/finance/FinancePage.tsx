import { Link } from 'react-router-dom'
import { useFinanceOverviewQuery } from '../bots/botsApi'
import { formatRial, toFaDigits } from '../../lib/utils'

function StatCard({ title, value, accent }: { title: string; value: string; accent?: boolean }) {
  return (
    <div className={`card p-5 ${accent ? 'bg-brand-600 text-white border-brand-600' : ''}`}>
      <div className={`text-sm ${accent ? 'text-brand-100 dark:text-brand-100' : 'text-slate-500 dark:text-slate-400'}`}>{title}</div>
      <div className="text-2xl font-bold mt-2">{value}</div>
    </div>
  )
}

export default function FinancePage() {
  const { data, isLoading, error } = useFinanceOverviewQuery()

  if (isLoading) return <div className="text-slate-500 dark:text-slate-400">در حال بارگذاری…</div>
  if (error) return <div className="text-red-600 dark:text-red-400">خطا در دریافت اطلاعات</div>
  if (!data) return null

  return (
    <div className="space-y-6">
      <h2 className="text-2xl font-bold">داشبورد و مالی</h2>

      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <StatCard title="درآمد کل سیستم" value={formatRial(data.total_revenue_rials)} accent />
        <StatCard title="کل کاربران" value={toFaDigits(data.total_users)} />
        <StatCard title="اشتراک‌های فعال" value={toFaDigits(data.active_subscriptions)} />
      </div>

      <div className="card overflow-hidden">
        <div className="px-5 py-4 border-b border-slate-200 dark:border-slate-700 font-semibold">درآمد به تفکیک بات</div>
        {data.per_bot.length === 0 ? (
          <div className="p-8 text-center text-slate-400 dark:text-slate-500">
            هنوز باتی ثبت نشده — از بخش «بات‌ها» یک بات بسازید.
          </div>
        ) : (
          <table className="w-full text-sm">
            <thead className="bg-slate-50 dark:bg-slate-900 text-slate-500 dark:text-slate-400">
              <tr>
                <th className="text-start px-5 py-3 font-medium">بات</th>
                <th className="text-start px-5 py-3 font-medium">درآمد</th>
                <th className="text-start px-5 py-3 font-medium">کاربران</th>
                <th className="text-start px-5 py-3 font-medium">اشتراک فعال</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
              {data.per_bot.map((row) => (
                <tr key={row.bot_id} className="hover:bg-slate-50 dark:hover:bg-slate-800">
                  <td className="px-5 py-3">
                    <Link to={`/bots/${row.bot_id}`} className="text-brand-600 dark:text-brand-500 hover:underline">
                      {row.bot_name}
                    </Link>
                  </td>
                  <td className="px-5 py-3">{formatRial(row.revenue_rials)}</td>
                  <td className="px-5 py-3">{toFaDigits(row.users)}</td>
                  <td className="px-5 py-3">{toFaDigits(row.active_subscriptions)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}
