import { useMemo, useState, type FormEvent } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import {
  useAdjustCoinsMutation,
  useApproveConfessionMutation,
  useBroadcastPreviewQuery,
  useCloseTicketMutation,
  useCloneBotMutation,
  useCreateDiscountMutation,
  useCreatePaymentMutation,
  useDeleteBotMutation,
  useDeleteDiscountMutation,
  useDirectMessageMutation,
  useGetBotQuery,
  useGetBotStatsQuery,
  useGetAIQuery,
  useGetAIDetailQuery,
  useGetChannelsQuery,
  useGetChatTranscriptQuery,
  useGetConfigQuery,
  useGetContentChatsQuery,
  useGetContentFoldersQuery,
  useGetContentPostsQuery,
  useGetContentSettingsQuery,
  useGetSettingsQuery,
  useGetUserDetailQuery,
  useListActiveChatsQuery,
  useListBlockPairsQuery,
  useListBroadcastsQuery,
  useListConfessionsQuery,
  useListDatingUsersQuery,
  useListDiscountsQuery,
  useListEventsQuery,
  useListPaymentsQuery,
  useListRecentChatMessagesQuery,
  useListStuckUsersQuery,
  useListTicketsQuery,
  usePutConfigMutation,
  usePutChannelsMutation,
  usePutContentSettingsMutation,
  usePutSettingsMutation,
  useRejectConfessionMutation,
  useReplyTicketMutation,
  useResetAllChatsMutation,
  useResetStreakMutation,
  useResetStuckMutation,
  useResetUserMutation,
  useScanTelegramBlocksMutation,
  useSeedConfessionsMutation,
  useSetAIEnabledMutation,
  useSetUserBlockedMutation,
  useStartBroadcastMutation,
  useStartBotMutation,
  useStopBotMutation,
  type ContentSettings,
  type ContentSettingsPut,
} from './botsApi'
import { GameSettingsTab, GamePlayersTab, WithdrawalsTab } from './GameTabs'
import { formatDateFa, formatRial, toFaDigits } from '../../lib/utils'
import { useTgListAccountsQuery } from '../telegram/telegramApi'

// Config fields arrive either as a JSON object (stored as jsonb) or as a
// JSON-encoded string (older rows); render both as pretty text.
function prettyJSON(value: unknown): string {
  if (value == null || value === '') return '{}'
  if (typeof value === 'string') {
    try {
      return JSON.stringify(JSON.parse(value), null, 2)
    } catch {
      return value
    }
  }
  return JSON.stringify(value, null, 2)
}

type Tab = 'users' | 'chats' | 'activity' | 'payments' | 'confessions' | 'tickets' | 'broadcast' | 'discounts' | 'health' | 'settings' | 'channels' | 'ai' | 'config' | 'content' | 'game' | 'players' | 'withdrawals'

const datingTabs: Array<{ key: Tab; label: string }> = [
  { key: 'users', label: '👥 کاربران' },
  { key: 'chats', label: '💬 چت‌ها' },
  { key: 'health', label: '🩺 سلامت و ریست' },
  { key: 'settings', label: '🎛 تنظیمات عددی' },
  { key: 'channels', label: '📢 کانال‌ها' },
  { key: 'ai', label: '🤖 دستیار هوشمند' },
  { key: 'activity', label: '📈 فعالیت' },
  { key: 'payments', label: '💰 پرداخت‌ها' },
  { key: 'confessions', label: '💌 اعتراف‌ها' },
  { key: 'tickets', label: '📩 تیکت‌ها' },
  { key: 'broadcast', label: '📣 اعلان' },
  { key: 'discounts', label: '🎟 کدهای تخفیف' },
  { key: 'config', label: '⚙️ تنظیمات و منو' },
]

const contentTabs: Array<{ key: Tab; label: string }> = [
  { key: 'content', label: '📗 محتوا و رخدادها' },
  { key: 'config', label: '⚙️ تنظیمات و منو' },
]

const gameTabs: Array<{ key: Tab; label: string }> = [
  { key: 'game', label: '🎮 تنظیمات بازی' },
  { key: 'players', label: '👥 بازیکنان' },
  { key: 'payments', label: '💰 پرداخت‌ها' },
  { key: 'config', label: '⚙️ تنظیمات و منو' },
]

const duelTabs: Array<{ key: Tab; label: string }> = [
  { key: 'game', label: '🎮 تنظیمات بازی' },
  { key: 'players', label: '👥 بازیکنان' },
  { key: 'withdrawals', label: '💎 برداشت‌ها' },
  { key: 'payments', label: '💰 پرداخت‌ها' },
  { key: 'config', label: '⚙️ تنظیمات و منو' },
]

// tabsFor returns the visible tab list for a bot's template.
function tabsFor(templateKey: string): Array<{ key: Tab; label: string }> {
  switch (templateKey) {
    case 'content': return contentTabs
    case 'bombland':
    case 'favaani':
    case 'tapearn':
      return gameTabs
    case 'duel':
      return duelTabs
    case 'shlinks':
    case 'whoseeme':
      return gameTabs
    case 'faal':
    case 'anonychat':
      return [{ key: 'game', label: '🎮 تنظیمات' }, { key: 'config', label: '⚙️ تنظیمات و منو' }]
    default: return datingTabs
  }
}

export default function BotDetailPage() {
  const { id = '' } = useParams()
  const [searchParams, setSearchParams] = useSearchParams()
  const navigate = useNavigate()
  const { data: botData, isLoading } = useGetBotQuery(id)
  const { data: stats } = useGetBotStatsQuery(id)
  const [startBot, { isLoading: starting }] = useStartBotMutation()
  const [stopBot, { isLoading: stopping }] = useStopBotMutation()
  const [deleteBot] = useDeleteBotMutation()
  const [showClone, setShowClone] = useState(false)

  if (isLoading || !botData) return <div className="text-slate-500 dark:text-slate-400">در حال بارگذاری…</div>
  const { bot, running } = botData
  const isContent = bot.template_key === 'content'
  const isDuel = bot.template_key === 'duel'
  const isGame = ['bombland','duel','favaani','shlinks','whoseeme','faal','anonychat','tapearn'].includes(bot.template_key)
  const tabs = tabsFor(bot.template_key)

  // active tab lives in the URL (?tab=...) so a refresh reopens it; unknown
  // or not-applicable tabs fall back to the first visible one
  const tabKey = searchParams.get('tab')
  const tab = tabs.some((t) => t.key === tabKey) ? (tabKey as Tab) : tabs[0].key
  const setTab = (next: Tab) => {
    const params = new URLSearchParams(searchParams)
    params.set('tab', next)
    setSearchParams(params, { replace: true })
  }

  const onDelete = async () => {
    if (!confirm(`بات «${bot.name}» حذف شود؟ تمام داده‌های آن پاک می‌شود.`)) return
    await deleteBot(id)
    window.location.href = '/bots'
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between flex-wrap gap-3">
        <div>
          <div className="text-sm text-slate-500 dark:text-slate-400">
            <Link to="/bots" className="hover:underline">بات‌ها</Link> / {bot.name}
          </div>
          <h2 className="text-2xl font-bold mt-1">
            {bot.name}{' '}
            <span className={`text-sm font-normal ${running ? 'text-green-600 dark:text-green-400' : 'text-slate-400 dark:text-slate-500'}`}>
              {running ? '● در حال اجرا' : '○ متوقف'}
            </span>
          </h2>
          {bot.telegram_username && (
            <div className="text-sm text-slate-500 dark:text-slate-400 font-mono" dir="ltr">{bot.telegram_username}</div>
          )}
          {bot.last_error && <div className="text-sm text-red-600 dark:text-red-400 mt-1">خطا: {bot.last_error}</div>}
        </div>
        <div className="flex gap-2">
          {running ? (
            <button className="btn-secondary" disabled={stopping} onClick={() => stopBot(id)}>
              توقف
            </button>
          ) : (
            <button className="btn-primary" disabled={starting} onClick={() => startBot(id)}>
              اجرا
            </button>
          )}
          {isContent && (
            <button className="btn-secondary" onClick={() => setShowClone(true)}>کلون بات</button>
          )}
          <button className="btn-danger" onClick={onDelete}>حذف بات</button>
        </div>
      </div>

      {showClone && (
        <CloneModal botId={id} botName={bot.name} onClose={() => setShowClone(false)}
          onDone={(newId, configError) => {
            setShowClone(false)
            if (configError) {
              alert(`بات ساخته شد ولی تنظیمات کپی نشد: ${configError}\nتنظیمات را دستی پر کنید.`)
            }
            navigate(`/bots/${newId}?tab=content`)
          }}
        />
      )}

      {stats && !isContent && (
        <div className="grid grid-cols-2 md:grid-cols-5 gap-3">
          <StatBox label="کاربران" value={toFaDigits(stats.total_users)} />
          <StatBox label="کاربر جدید (۳۰ روز)" value={toFaDigits(stats.new_users_30d)} />
          <StatBox label="درآمد ۳۰ روز" value={formatRial(stats.revenue_30d_rials)} />
          <StatBox label="اشتراک فعال" value={toFaDigits(stats.active_subscriptions)} />
          <StatBox label="رویداد ۲۴ ساعت" value={toFaDigits(stats.events_24h)} />
        </div>
      )}

      <div className="border-b border-slate-200 dark:border-slate-700 flex gap-1">
        {tabs.map((t) => (
          <button
            key={t.key}
            onClick={() => setTab(t.key)}
            className={`px-4 py-2 text-sm font-medium rounded-t-lg ${
              tab === t.key
                ? 'bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-700 border-b-white text-brand-600 dark:text-brand-500 -mb-px'
                : 'text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200'
            }`}
          >
            {t.label}
          </button>
        ))}
      </div>

      {tab === 'content' && <ContentTab botId={id} onClone={() => setShowClone(true)} />}
      {isGame && tab === 'game' && <GameSettingsTab botId={id} templateKey={bot.template_key} />}
      {isGame && tab === 'players' && <GamePlayersTab templateKey={bot.template_key} />}
      {isDuel && tab === 'withdrawals' && <WithdrawalsTab botId={id} />}
      {!isContent && !isGame && tab === 'users' && <UsersTab botId={id} />}
      {!isContent && !isGame && tab === 'chats' && <ChatsTab botId={id} />}
      {!isContent && !isGame && tab === 'health' && <HealthTab botId={id} />}
      {!isContent && !isGame && tab === 'settings' && <SettingsTab botId={id} />}
      {!isContent && !isGame && tab === 'channels' && <ChannelsTab botId={id} />}
      {!isContent && !isGame && tab === 'ai' && <AITab botId={id} />}
      {!isContent && !isGame && tab === 'activity' && <ActivityTab botId={id} />}
      {tab === 'payments' && <PaymentsTab botId={id} />}
      {!isContent && !isGame && tab === 'confessions' && <ConfessionsTab botId={id} />}
      {!isContent && !isGame && tab === 'tickets' && <TicketsTab botId={id} />}
      {!isContent && !isGame && tab === 'broadcast' && <BroadcastTab botId={id} />}
      {!isContent && !isGame && tab === 'discounts' && <DiscountsTab botId={id} />}
      {tab === 'config' && <ConfigTab botId={id} />}
    </div>
  )
}

function ConfessionsTab({ botId }: { botId: string }) {
  const [status, setStatus] = useState('pending')
  const [page, setPage] = useState(1)
  const { data } = useListConfessionsQuery({ botId, status, page })
  const [approve] = useApproveConfessionMutation()
  const [reject] = useRejectConfessionMutation()
  const [seed, { isLoading: seeding }] = useSeedConfessionsMutation()

  const onSeed = async () => {
    if (!confirm('۲۴ اعتراف نمونه به‌صورت «در انتظار بررسی» درج شود؟ (فقط با تایید شما در کانال می‌رود)')) return
    await seed(botId).unwrap()
  }

  return (
    <div className="space-y-4">
      <div className="flex gap-2 flex-wrap">
        {[
          { key: 'pending', label: 'در انتظار تایید' },
          { key: 'approved', label: 'تاییدشده' },
          { key: 'rejected', label: 'ردشده' },
        ].map((s) => (
          <button
            key={s.key}
            className={`px-3 py-1.5 rounded-lg text-sm ${status === s.key ? 'bg-brand-600 text-white' : 'bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-300'}`}
            onClick={() => { setStatus(s.key); setPage(1) }}
          >
            {s.label}
          </button>
        ))}
        <div className="flex-1" />
        <button className="btn-secondary px-3 py-1.5 text-sm" disabled={seeding} onClick={onSeed}>
          {seeding ? 'در حال درج…' : '📥 درج اعتراف‌های نمونه'}
        </button>
      </div>
      <div className="space-y-3">
        {data?.confessions.map((c) => (
          <div key={c.id} className="card p-4">
            <div className="text-slate-700 dark:text-slate-200 whitespace-pre-wrap">{c.text}</div>
            <div className="flex items-center justify-between mt-3">
              <div className="text-xs text-slate-400 dark:text-slate-500">{c.username ? `@${c.username}` : c.first_name ? c.first_name : `کاربر #${c.user_id}`} · {formatDateFa(c.created_at)}</div>
              {c.status === 'pending' && (
                <div className="flex gap-2">
                  <button className="btn-primary px-3 py-1 text-sm" onClick={() => approve({ botId, id: c.id })}>
                    تایید و انتشار
                  </button>
                  <button className="btn-danger px-3 py-1 text-sm" onClick={() => reject({ botId, id: c.id })}>
                    رد
                  </button>
                </div>
              )}
            </div>
          </div>
        ))}
        {data && data.confessions.length === 0 && (
          <div className="card p-8 text-center text-slate-400 dark:text-slate-500">اعترافی در این وضعیت نیست</div>
        )}
      </div>
      <Paginator page={page} total={data?.total ?? 0} onPage={setPage} />
    </div>
  )
}

function TicketsTab({ botId }: { botId: string }) {
  const [status, setStatus] = useState('open')
  const [page, setPage] = useState(1)
  const { data } = useListTicketsQuery({ botId, status, page })
  const [reply] = useReplyTicketMutation()
  const [close] = useCloseTicketMutation()
  const [drafts, setDrafts] = useState<Record<number, string>>({})

  return (
    <div className="space-y-4">
      <div className="flex gap-2">
        {[
          { key: 'open', label: 'باز' },
          { key: 'answered', label: 'پاسخ‌داده‌شده' },
          { key: 'closed', label: 'بسته' },
        ].map((s) => (
          <button
            key={s.key}
            className={`px-3 py-1.5 rounded-lg text-sm ${status === s.key ? 'bg-brand-600 text-white' : 'bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-300'}`}
            onClick={() => { setStatus(s.key); setPage(1) }}
          >
            {s.label}
          </button>
        ))}
      </div>
      <div className="space-y-3">
        {data?.tickets.map((t) => (
          <div key={t.id} className="card p-4">
            <div className="text-slate-700 dark:text-slate-200">{t.message}</div>
            {t.admin_reply && (
              <div className="mt-2 p-3 bg-slate-50 dark:bg-slate-900 rounded-lg text-sm text-slate-600 dark:text-slate-300 border-s-2 border-brand-400">
                پاسخ ادمین: {t.admin_reply}
              </div>
            )}
            <div className="flex items-center justify-between mt-3 gap-3 flex-wrap">
              <div className="text-xs text-slate-400 dark:text-slate-500">{t.username ? `@${t.username}` : t.first_name ? t.first_name : `کاربر #${t.user_id}`} · {formatDateFa(t.created_at)}</div>
              <div className="flex gap-2 items-center">
                {t.status !== 'closed' && (
                  <>
                    <input
                      className="input flex-1 min-w-48 text-sm"
                      placeholder="پاسخ به کاربر…"
                      value={drafts[t.id] ?? ''}
                      onChange={(e) => setDrafts({ ...drafts, [t.id]: e.target.value })}
                    />
                    <button
                      className="btn-primary px-3 py-1 text-sm"
                      disabled={!(drafts[t.id] ?? '').trim()}
                      onClick={() => reply({ botId, id: t.id, reply: drafts[t.id] }).then(() => setDrafts({ ...drafts, [t.id]: '' }))}
                    >
                      ارسال پاسخ
                    </button>
                    <button className="btn-secondary px-3 py-1 text-sm" onClick={() => close({ botId, id: t.id })}>
                      بستن
                    </button>
                  </>
                )}
              </div>
            </div>
          </div>
        ))}
        {data && data.tickets.length === 0 && (
          <div className="card p-8 text-center text-slate-400 dark:text-slate-500">تیکتی در این وضعیت نیست</div>
        )}
      </div>
      <Paginator page={page} total={data?.total ?? 0} onPage={setPage} />
    </div>
  )
}

function StatBox({ label, value }: { label: string; value: string }) {
  return (
    <div className="card p-4">
      <div className="text-xs text-slate-500 dark:text-slate-400">{label}</div>
      <div className="font-bold mt-1 text-sm">{value}</div>
    </div>
  )
}

function UsersTab({ botId }: { botId: string }) {
  const [q, setQ] = useState('')
  const [page, setPage] = useState(1)
  const [ghostOnly, setGhostOnly] = useState(false)
  const [detailFor, setDetailFor] = useState<number | null>(null)
  const { data } = useListDatingUsersQuery({ botId, q, page, ghost: ghostOnly ? 'only' : undefined })
  const statusLabels: Record<string, string> = {
    joined: 'در منو',
    searching: 'در جستجو',
    chatting: 'در چت',
  }
  // presence: online <5m, semi-online <60m, offline older
  const presence = (lastSeen: string) => {
    const mins = (Date.now() - new Date(lastSeen).getTime()) / 60000
    if (mins < 5) return { dot: 'bg-green-50 dark:bg-green-9500', label: 'آنلاین' }
    if (mins < 60) return { dot: 'bg-amber-400', label: 'نیمه‌آنلاین' }
    return { dot: 'bg-slate-300 dark:bg-slate-600', label: 'آفلاین' }
  }
  return (
    <div className="card">
      <div className="p-4 border-b border-slate-200 dark:border-slate-700 flex flex-wrap items-center gap-3">
        <input
          className="input max-w-sm"
          placeholder="جستجو: username، نام یا telegram_id…"
          value={q}
          onChange={(e) => { setQ(e.target.value); setPage(1) }}
        />
        <button
          className={`text-sm px-3 py-1.5 rounded-lg border ${ghostOnly ? 'bg-violet-50 dark:bg-violet-950 border-violet-300 dark:border-violet-700 text-violet-700 dark:text-violet-400' : 'border-slate-200 dark:border-slate-700 text-slate-500 dark:text-slate-400'}`}
          onClick={() => { setGhostOnly((g) => !g); setPage(1) }}
        >
          👻 فقط جعلی‌ها
        </button>
      </div>
      <table className="w-full text-sm">
        <thead className="bg-slate-50 dark:bg-slate-900 text-slate-500 dark:text-slate-400">
          <tr>
            <th className="text-start px-4 py-3">ID تلگرام</th>
            <th className="text-start px-4 py-3">نام</th>
            <th className="text-start px-4 py-3">username</th>
            <th className="text-start px-4 py-3">سکه</th>
            <th className="text-start px-4 py-3">وضعیت</th>
            <th className="text-start px-4 py-3">امتیاز</th>
            <th className="text-start px-4 py-3">حضور</th>
            <th className="text-start px-4 py-3">آخرین فعالیت</th>
            <th className="text-start px-4 py-3"></th>
          </tr>
        </thead>
        <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
          {data?.users.map((u) => {
            const p = presence(u.last_seen_at)
            return (
            <tr key={u.user_id} className="hover:bg-slate-50 dark:hover:bg-slate-800">
              <td className="px-4 py-2 font-mono text-xs" dir="ltr">{u.telegram_id}</td>
              <td className="px-4 py-2">
                {u.first_name ?? '—'}
                {u.is_ghost && (
                  <span className="ms-1 text-xs text-violet-500 dark:text-violet-400" title={u.ghost_persona ?? undefined}>👻</span>
                )}
              </td>
              <td className="px-4 py-2 font-mono text-xs" dir="ltr">{u.username ? `@${u.username}` : '—'}</td>
              <td className="px-4 py-2">{u.coins != null ? toFaDigits(u.coins) : '—'}</td>
              <td className="px-4 py-2">{u.status ? (statusLabels[u.status] ?? u.status) : '—'}</td>
              <td className="px-4 py-2">
                {u.total_ratings
                  ? `${toFaDigits(Number(u.rating ?? 0).toFixed(1))} (${toFaDigits(u.total_ratings)})`
                  : '—'}
              </td>
              <td className="px-4 py-2 whitespace-nowrap">
                <span className={`inline-block w-2 h-2 rounded-full ms-1 ${p.dot}`} />
                {p.label}
              </td>
              <td className="px-4 py-2 text-slate-500 dark:text-slate-400">{formatDateFa(u.last_seen_at)}</td>
              <td className="px-4 py-2">
                <button className="btn-ghost text-xs" onClick={() => setDetailFor(u.user_id)}>
                  جزئیات
                </button>
              </td>
            </tr>
            )
          })}
          {data && data.users.length === 0 && (
            <tr><td colSpan={9} className="p-8 text-center text-slate-400 dark:text-slate-500">کاربری یافت نشد</td></tr>
          )}
        </tbody>
      </table>
      <Paginator page={page} total={data?.total ?? 0} onPage={setPage} />
      {detailFor != null && (
        <UserDetailDrawer botId={botId} userId={detailFor} onClose={() => setDetailFor(null)} />
      )}
    </div>
  )
}

// UserDetailDrawer is the per-user panel: coins, reset, block, streak, and a
// direct message, plus payments and recent activity.
function UserDetailDrawer({ botId, userId, onClose }: { botId: string; userId: number; onClose: () => void }) {
  const { data: d, isLoading } = useGetUserDetailQuery({ botId, userId })
  const [adjustCoins, { isLoading: adjusting }] = useAdjustCoinsMutation()
  const [resetUser, { isLoading: resetting }] = useResetUserMutation()
  const [setBlocked, { isLoading: blocking }] = useSetUserBlockedMutation()
  const [resetStreak, { isLoading: streakBusy }] = useResetStreakMutation()
  const [directMessage, { isLoading: sending }] = useDirectMessageMutation()
  const [delta, setDelta] = useState('10')
  const [reason, setReason] = useState('')
  const [msg, setMsg] = useState('')
  const [note, setNote] = useState<string | null>(null)

  const act = async (fn: () => Promise<unknown>, okMsg: string) => {
    setNote(null)
    try {
      await fn()
      setNote(okMsg)
    } catch {
      setNote('عملیات ناموفق بود')
    }
  }

  return (
    <div className="fixed inset-0 bg-black/30 flex justify-end z-50" onClick={onClose}>
      <div
        className="bg-white dark:bg-slate-900 w-full sm:max-w-lg h-full overflow-y-auto shadow-xl p-5 space-y-4"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between">
          <h3 className="font-bold">جزئیات کاربر</h3>
          <button className="btn-ghost" onClick={onClose}>بستن</button>
        </div>

        {isLoading || !d ? (
          <div className="text-slate-500 dark:text-slate-400">در حال بارگذاری…</div>
        ) : (
          <>
            <div className="card p-4 text-sm space-y-1">
              <div>{d.first_name ?? '—'} {d.username ? `(@${d.username})` : ''}</div>
              <div className="font-mono text-xs text-slate-500 dark:text-slate-400" dir="ltr">{d.telegram_id}</div>
              <div className="flex gap-3 pt-1">
                <span>سکه: {toFaDigits(d.coins ?? 0)}</span>
                <span>وضعیت: {d.status ?? '—'}</span>
                <span>استریک: {toFaDigits(d.streak_count ?? 0)}</span>
              </div>
              <div className="flex gap-3 text-slate-500 dark:text-slate-400">
                <span>لایک فرستاده: {toFaDigits(d.likes_sent)}</span>
                <span>دریافتی: {toFaDigits(d.likes_received)}</span>
              </div>
              {d.profile_blocked_at && (
                <div className="text-red-600 dark:text-red-400">
                  🚫 مسدود{d.blocked_reason ? `: ${d.blocked_reason}` : ''}
                </div>
              )}
              {!d.has_profile && <div className="text-amber-600 dark:text-amber-400">پروفایل دوستیابی ندارد</div>}
            </div>

            <div className="card p-4 space-y-2">
              <div className="text-sm font-semibold">سکه</div>
              <div className="flex gap-2">
                <input className="input max-w-[9rem]" dir="ltr" value={delta} onChange={(e) => setDelta(e.target.value)} />
                <button
                  className="btn-ghost"
                  disabled={adjusting}
                  onClick={() => act(() => adjustCoins({ botId, userId, delta: Number(delta) }).unwrap(), 'سکه اعمال شد')}
                >
                  اعمال (±)
                </button>
                <button
                  className="btn-ghost"
                  disabled={adjusting}
                  onClick={() => act(() => adjustCoins({ botId, userId, delta: 50 }).unwrap(), '۵۰ سکه اضافه شد')}
                >
                  +۵۰
                </button>
              </div>
            </div>

            <div className="card p-4 space-y-2">
              <div className="text-sm font-semibold">دسترسی</div>
              <div className="flex flex-wrap gap-2">
                <button
                  className="btn-ghost"
                  disabled={resetting}
                  onClick={() => {
                    if (!confirm('ریست کامل: سکه، لایک‌ها، امتیاز و وضعیت چت پاک می‌شود. مطمئنید؟')) return
                    act(() => resetUser({ botId, userId }).unwrap(), 'کاربر ریست شد')
                  }}
                >
                  ریست کاربر
                </button>
                <button
                  className="btn-ghost"
                  disabled={streakBusy}
                  onClick={() => act(() => resetStreak({ botId, userId }).unwrap(), 'استریک صفر شد')}
                >
                  ریست استریک
                </button>
                <button
                  className="btn-ghost"
                  disabled={blocking}
                  onClick={() =>
                    act(
                      () => setBlocked({ botId, userId, blocked: !d.profile_blocked_at, reason }).unwrap(),
                      d.profile_blocked_at ? 'رفع مسدودی شد' : 'کاربر مسدود شد',
                    )
                  }
                >
                  {d.profile_blocked_at ? 'رفع مسدودی' : 'مسدود کردن'}
                </button>
              </div>
              {!d.profile_blocked_at && (
                <input
                  className="input"
                  placeholder="دلیل مسدودسازی (اختیاری)"
                  value={reason}
                  onChange={(e) => setReason(e.target.value)}
                />
              )}
            </div>

            <div className="card p-4 space-y-2">
              <div className="text-sm font-semibold">ارسال پیام از طرف بات</div>
              <textarea
                className="input h-24"
                value={msg}
                onChange={(e) => setMsg(e.target.value)}
                placeholder="متن پیام…"
              />
              <button
                className="btn-ghost"
                disabled={sending || !msg.trim()}
                onClick={() => act(async () => {
                  await directMessage({ botId, userId, text: msg }).unwrap()
                  setMsg('')
                }, 'پیام ارسال شد')}
              >
                ارسال
              </button>
            </div>

            {note && <div className="text-sm text-slate-600 dark:text-slate-300 bg-slate-50 dark:bg-slate-900 rounded-lg p-3">{note}</div>}

            <div className="card p-4">
              <div className="text-sm font-semibold mb-2">پرداخت‌های اخیر</div>
              {d.payments.length === 0 ? (
                <div className="text-slate-400 dark:text-slate-500 text-sm">پرداختی ثبت نشده</div>
              ) : (
                <table className="w-full text-xs">
                  <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                    {d.payments.map((p) => (
                      <tr key={p.id}>
                        <td className="py-1">{formatRial(p.amount_rials)}</td>
                        <td className="py-1">{p.kind}</td>
                        <td className="py-1">{p.status}</td>
                        <td className="py-1 text-slate-500 dark:text-slate-400">{formatDateFa(p.created_at)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </div>

            <div className="card p-4">
              <div className="text-sm font-semibold mb-2">فعالیت اخیر</div>
              {d.recent_events.length === 0 ? (
                <div className="text-slate-400 dark:text-slate-500 text-sm">رویدادی ثبت نشده</div>
              ) : (
                <ul className="text-xs space-y-1">
                  {d.recent_events.map((e) => (
                    <li key={e.id} className="flex justify-between gap-2">
                      <span className="text-slate-500 dark:text-slate-400">{e.kind}</span>
                      <span className="truncate">{e.text ?? '—'}</span>
                      <span className="text-slate-400 dark:text-slate-500 shrink-0">{formatDateFa(e.created_at)}</span>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          </>
        )}
      </div>
    </div>
  )
}

// ChatsTab is the chat surveillance tool: every live conversation, a
// reconstructed transcript per pair, and a global recent-message feed.
function ChatsTab({ botId }: { botId: string }) {
  const { data: chats } = useListActiveChatsQuery(botId, { pollingInterval: 8000 })
  const { data: feed } = useListRecentChatMessagesQuery(botId, { pollingInterval: 8000 })
  const [open, setOpen] = useState<{ a: number; b: number; label: string } | null>(null)

  const name = (n?: string, u?: string) => n ?? (u ? `@${u}` : '—')

  if (open) {
    return <TranscriptPanel botId={botId} pair={open} onBack={() => setOpen(null)} />
  }

  return (
    <div className="space-y-4">
      <div className="card">
        <div className="p-4 border-b border-slate-200 dark:border-slate-700 font-semibold">
          💬 چت‌های فعال {chats ? `(${toFaDigits(chats.total)})` : ''}
        </div>
        <table className="w-full text-sm">
          <thead className="bg-slate-50 dark:bg-slate-900 text-slate-500 dark:text-slate-400">
            <tr>
              <th className="text-start px-4 py-3">کاربر ۱</th>
              <th className="text-start px-4 py-3">کاربر ۲</th>
              <th className="text-start px-4 py-3">وضعیت</th>
              <th className="text-start px-4 py-3">آخرین فعالیت</th>
              <th className="text-start px-4 py-3"></th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
            {chats?.chats.map((c) => (
              <tr key={`${c.user_a_id}-${c.user_b_id}`} className="hover:bg-slate-50 dark:hover:bg-slate-800">
                <td className="px-4 py-2">
                  {name(c.name_a, c.username_a)}
                  {c.is_ghost_a && <span className="ms-1 text-violet-500 dark:text-violet-400">👻</span>}
                  <span className="ms-2 font-mono text-xs text-slate-400 dark:text-slate-500" dir="ltr">{c.telegram_a_id}</span>
                </td>
                <td className="px-4 py-2">
                  {name(c.name_b, c.username_b)}
                  {c.is_ghost_b && <span className="ms-1 text-violet-500 dark:text-violet-400">👻</span>}
                  <span className="ms-2 font-mono text-xs text-slate-400 dark:text-slate-500" dir="ltr">{c.telegram_b_id}</span>
                </td>
                <td className="px-4 py-2">{c.status === 'chatting' ? 'در چت' : c.status}</td>
                <td className="px-4 py-2 text-slate-500 dark:text-slate-400">{formatDateFa(c.last_seen_at)}</td>
                <td className="px-4 py-2">
                  <button
                    className="btn-ghost text-xs"
                    onClick={() => setOpen({
                      a: c.user_a_id,
                      b: c.user_b_id,
                      label: `${name(c.name_a, c.username_a)} ← ${name(c.name_b, c.username_b)}`,
                    })}
                  >
                    مشاهدهٔ مکالمه
                  </button>
                </td>
              </tr>
            ))}
            {chats && chats.chats.length === 0 && (
              <tr><td colSpan={5} className="p-8 text-center text-slate-400 dark:text-slate-500">چت فعالی در جریان نیست</td></tr>
            )}
            {!chats && <tr><td colSpan={5} className="p-8 text-center text-slate-400 dark:text-slate-500">در حال بارگذاری…</td></tr>}
          </tbody>
        </table>
      </div>

      <div className="card">
        <div className="p-4 border-b border-slate-200 dark:border-slate-700 font-semibold">
          🕐 آخرین پیام‌های چت {feed ? `(${toFaDigits(feed.total)})` : ''}
        </div>
        <div className="max-h-96 overflow-y-auto">
          <table className="w-full text-sm">
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
              {feed?.messages.map((m) => (
                <tr key={m.event_id} className="hover:bg-slate-50 dark:hover:bg-slate-800">
                  <td className="px-3 py-2 w-48">
                    <div className="flex items-center gap-1.5 flex-wrap">
                      <span className="font-medium text-xs">{m.username ? `@${m.username}` : m.name ?? '—'}</span>
                      <span className={`text-[10px] px-1.5 py-0.5 rounded ${
                        m.is_outbound
                          ? 'bg-blue-100 text-blue-600 dark:bg-blue-900/30 dark:text-blue-400'
                          : 'bg-green-100 text-green-600 dark:bg-green-900/30 dark:text-green-400'
                      }`}>
                        {m.is_outbound ? 'ارسالی' : 'دریافتی'}
                      </span>
                    </div>
                    {m.partner_user_id && (
                      <div className="text-[11px] text-slate-400 dark:text-slate-500 mt-0.5">
                        به/از {m.partner_username ? `@${m.partner_username}` : m.partner_name ?? `#${m.partner_user_id}`}
                      </div>
                    )}
                  </td>
                  <td className="px-3 py-2 text-sm">{m.text}</td>
                  <td className="px-3 py-2 text-xs text-slate-400 dark:text-slate-500 whitespace-nowrap w-32">{formatDateFa(m.created_at)}</td>
                </tr>
              ))}
              {feed && feed.messages.length === 0 && (
                <tr><td colSpan={3} className="p-8 text-center text-slate-400 dark:text-slate-500">هنوز پیامی در چتها ثبت نشده</td></tr>
              )}
              {!feed && <tr><td colSpan={3} className="p-8 text-center text-slate-400 dark:text-slate-500">در حال بارگذاری…</td></tr>}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}

// TranscriptPanel renders one reconstructed conversation oldest-first.
function TranscriptPanel({ botId, pair, onBack }: {
  botId: string
  pair: { a: number; b: number; label: string }
  onBack: () => void
}) {
  const { data, isLoading, error, refetch, isFetching } = useGetChatTranscriptQuery(
    { botId, a: pair.a, b: pair.b },
    { pollingInterval: 5000 },
  )
  const errMsg = error && typeof error === 'object' && 'data' in error
    ? ((error.data as { error?: string })?.error ?? 'خطا در دریافت مکالمه')
    : null

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-3">
        <button className="btn-ghost" onClick={onBack}>→ بازگشت</button>
        <div className="font-semibold">{pair.label}</div>
        <button className="btn-ghost text-xs" onClick={() => refetch()}>
          {isFetching ? 'تازه‌سازی…' : '🔄 بروزرسانی'}
        </button>
      </div>

      <div className="card p-4">
        <div className="text-sm text-slate-500 dark:text-slate-400 mb-3">
          فقط پیام‌هایی که کاربران در صحنهٔ چت نوشته‌اند ثبت می‌شود؛ پیام‌هایی که در
          منو، جستجو یا ثبت‌نام فرستاده شده در اینجا نمایش داده نمی‌شوند.
        </div>
        {isLoading && <div className="text-slate-500 dark:text-slate-400">در حال بارگذاری…</div>}
        {errMsg && <div className="text-red-600 dark:text-red-400">{errMsg}</div>}

        <ul className="space-y-2 max-h-[32rem] overflow-y-auto my-3">
          {data?.messages.map((m) => (
            <li key={m.event_id} className={`flex ${m.is_user_a ? 'justify-start' : 'justify-end'}`}>
              <div className={`max-w-[80%] rounded-lg px-3 py-2 ${
                m.is_user_a ? 'bg-blue-50 border-blue-100' : 'bg-emerald-50 dark:bg-emerald-950 border-emerald-100 dark:border-emerald-800'
              } border`}>
                <div className="text-[11px] text-slate-500 dark:text-slate-400 mb-1 flex justify-between gap-3">
                  <span>{m.name} <span className="font-mono" dir="ltr">({m.telegram_id})</span></span>
                  <span dir="ltr">{formatDateFa(m.created_at)}</span>
                </div>
                <div className="text-sm">{m.text}</div>
              </div>
            </li>
          ))}
          {data && data.messages.length === 0 && (
            <li className="text-slate-400 dark:text-slate-500 text-center py-6">مکالمه‌ای در این چت ثبت نشده است</li>
          )}
        </ul>

        {data && data.messages.length > 0 && (
          <div className="text-xs text-slate-500 dark:text-slate-400">
            {toFaDigits(data.total)} پیام
          </div>
        )}
      </div>
    </div>
  )
}

// HealthTab shows stuck users and the bulk chat resets.
function HealthTab({ botId }: { botId: string }) {
  const { data, isFetching } = useListStuckUsersQuery(botId)
  const [resetStuck, { isLoading: resettingStuck }] = useResetStuckMutation()
  const [resetAll, { isLoading: resettingAll }] = useResetAllChatsMutation()
  const [scanBlocks, { isLoading: scanning }] = useScanTelegramBlocksMutation()
  const { data: blockPairs, refetch: refetchBlocks } = useListBlockPairsQuery(botId)
  const [scanDays, setScanDays] = useState('7')
  const [scanNote, setScanNote] = useState<string | null>(null)
  const [note, setNote] = useState<string | null>(null)

  const statusLabels: Record<string, string> = { searching: 'در جستجو', chatting: 'در چت' }

  return (
    <div className="space-y-4">
      <div className="card p-4 space-y-3">
        <div className="text-sm text-slate-500 dark:text-slate-400">
          کاربرانی که بیش از حد معمول در وضعیت جستجو یا چت مانده‌اند. ریست، آن‌ها و پارتنرهای منتظرشان را آزاد می‌کند.
        </div>
        <div className="flex flex-wrap gap-2">
          <button
            className="btn-ghost"
            disabled={resettingStuck}
            onClick={async () => {
              try {
                const r = await resetStuck(botId).unwrap()
                setNote(`${toFaDigits(r.reset)} کاربر آزاد شد`)
              } catch {
                setNote('ریست ناموفق بود')
              }
            }}
          >
            ریست گیرکرده‌ها
          </button>
          <button
            className="btn-ghost text-red-600 dark:text-red-400"
            disabled={resettingAll}
            onClick={async () => {
              if (!confirm('همهٔ چت‌ها و جستجوهای فعال بسته می‌شوند (سکه و پروفایل دست‌نخورده می‌ماند). مطمئنید؟')) return
              try {
                const r = await resetAll(botId).unwrap()
                setNote(`${toFaDigits(r.reset)} کاربر به حالت عادی برگشت`)
              } catch {
                setNote('ریست ناموفق بود')
              }
            }}
          >
            ریست همهٔ کاربران
          </button>
        </div>
        {note && <div className="text-sm text-slate-600 dark:text-slate-300 bg-slate-50 dark:bg-slate-900 rounded-lg p-3">{note}</div>}
      </div>

      <div className="card p-4 space-y-3">
        <div className="font-semibold">🚫 اسکن بلاک تلگرامی</div>
        <div className="text-sm text-slate-500 dark:text-slate-400">
          به کاربرانی که بیش از این تعداد روز غیرفعال بوده‌اند پیام آزمایشی می‌فرستد؛ پاسخ 403 (بات را بلاک کرده‌اند)
          ثبت می‌شود و این کاربران از اعلان‌ها حذف می‌شوند. بات باید در حال اجرا باشد.
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <label className="text-sm">غیرفعال از (روز):</label>
          <input className="input w-20" dir="ltr" type="number" min={1} max={365} value={scanDays} onChange={(e) => setScanDays(e.target.value)} />
          <button
            className="btn-secondary text-sm"
            disabled={scanning}
            onClick={async () => {
              setScanNote(null)
              try {
                const r = await scanBlocks({ botId, days: Number(scanDays) || 7 }).unwrap()
                setScanNote(`اسکن تمام شد: ${toFaDigits(r.scanned)} بررسی‌شده، ${toFaDigits(r.blocked)} بلاک، ${toFaDigits(r.reachable)} در دسترس، ${toFaDigits(r.failed)} خطا`)
                void refetchBlocks()
              } catch (e) {
                const msg = e && typeof e === 'object' && 'data' in e
                  ? ((e.data as { error?: string })?.error ?? 'اسکن ناموفق بود')
                  : 'اسکن ناموفق بود'
                setScanNote(msg)
              }
            }}
          >
            {scanning ? 'در حال اسکن…' : 'شروع اسکن'}
          </button>
          {scanNote && <span className="text-sm text-slate-600 dark:text-slate-300">{scanNote}</span>}
        </div>
      </div>

      <div className="card p-4 space-y-3">
        <div className="font-semibold">🧱 روابط بلاک کاربران ({toFaDigits(blockPairs?.total ?? 0)})</div>
        <div className="text-sm text-slate-500 dark:text-slate-400">کاربرانی که یکدیگر را در چت بلاک کرده‌اند (فقط روابط فعال).</div>
        {blockPairs?.blocks && blockPairs.blocks.length > 0 ? (
          <table className="w-full text-sm">
            <thead className="bg-slate-50 dark:bg-slate-900 text-slate-500 dark:text-slate-400">
              <tr>
                <th className="text-start px-3 py-2">بلاک‌کننده</th>
                <th className="text-start px-3 py-2">بلاک‌شده</th>
                <th className="text-start px-3 py-2">زمان</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
              {blockPairs.blocks.map((b, i) => (
                <tr key={i}>
                  <td className="px-3 py-2">{b.blocker_name} <span className="text-xs text-slate-400 dark:text-slate-500 font-mono" dir="ltr">({b.blocker_id})</span></td>
                  <td className="px-3 py-2">{b.blocked_name} <span className="text-xs text-slate-400 dark:text-slate-500 font-mono" dir="ltr">({b.blocked_id})</span></td>
                  <td className="px-3 py-2 text-slate-500 dark:text-slate-400">{formatDateFa(b.created_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        ) : (
          <div className="text-sm text-slate-400 dark:text-slate-500">روابط بلاک فعالی ثبت نشده.</div>
        )}
      </div>

      <div className="card">
        <table className="w-full text-sm">
          <thead className="bg-slate-50 dark:bg-slate-900 text-slate-500 dark:text-slate-400">
            <tr>
              <th className="text-start px-4 py-3">ID تلگرام</th>
              <th className="text-start px-4 py-3">نام</th>
              <th className="text-start px-4 py-3">وضعیت</th>
              <th className="text-start px-4 py-3">مدت (دقیقه)</th>
              <th className="text-start px-4 py-3">آخرین فعالیت</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
            {data?.users.map((u) => (
              <tr key={u.user_id} className="hover:bg-slate-50 dark:hover:bg-slate-800">
                <td className="px-4 py-2 font-mono text-xs" dir="ltr">{u.telegram_id}</td>
                <td className="px-4 py-2">{u.name ?? u.username ?? '—'}</td>
                <td className="px-4 py-2">{statusLabels[u.status] ?? u.status}</td>
                <td className="px-4 py-2">{toFaDigits(u.stuck_minutes)}</td>
                <td className="px-4 py-2 text-slate-500 dark:text-slate-400">{formatDateFa(u.last_seen_at)}</td>
              </tr>
            ))}
            {data && data.users.length === 0 && (
              <tr><td colSpan={5} className="p-8 text-center text-slate-400 dark:text-slate-500">
                {isFetching ? 'در حال بررسی…' : 'کاربر گیرکرده‌ای نیست ✅'}
              </td></tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}

// SettingsTab is the typed numeric form over the bot's price keys.
function ChannelsTab({ botId }: { botId: string }) {
  const { data: cfg } = useGetConfigQuery(botId)
  const { data, isLoading } = useGetChannelsQuery(botId)
  const [putChannels, { isLoading: saving }] = usePutChannelsMutation()
  const [rows, setRows] = useState<Array<{ id: string; title: string; link: string }> | null>(null)
  const [note, setNote] = useState<string | null>(null)

  const current = rows ?? (data?.channels ?? []).map((c) => ({ id: c.id, title: c.title ?? '', link: c.link ?? '' }))
  const dirty = JSON.stringify(current) !== JSON.stringify((data?.channels ?? []).map((c) => ({ id: c.id, title: c.title ?? '', link: c.link ?? '' })))

  const setRow = (i: number, patch: Partial<{ id: string; title: string; link: string }>) =>
    setRows(current.map((r, idx) => (idx === i ? { ...r, ...patch } : r)))

  const save = async () => {
    setNote(null)
    try {
      await putChannels({
        botId,
        version: cfg?.version ?? 0,
        channels: current.filter((r) => r.id.trim() !== '').map((r) => ({
          id: r.id.trim(),
          ...(r.title.trim() ? { title: r.title.trim() } : {}),
          ...(r.link.trim() ? { link: r.link.trim() } : {}),
        })),
      }).unwrap()
      setRows(null)
      setNote('ذخیره شد. کاربران برای دریافت جایزه باید عضو همه کانال‌ها باشند.')
    } catch (e) {
      const msg = e && typeof e === 'object' && 'data' in e
        ? ((e.data as { error?: string })?.error ?? 'خطا در ذخیره')
        : 'خطا در ذخیره'
      setNote(msg)
    }
  }

  if (isLoading) return <div className="text-slate-500 dark:text-slate-400">در حال بارگذاری…</div>

  return (
    <div className="space-y-4">
      <div className="text-sm text-slate-500 dark:text-slate-400">
        کانال‌های عضویت اجباری (ماموریت «عضویت کانال»): کاربر باید عضو همهٔ این کانال‌ها باشد تا ۱۰ سکه بگیرد.
        شناسه می‌تواند `@username` یا عددی `-100…` باشد.
      </div>
      <div className="card p-4 space-y-3">
        {current.length === 0 && (
          <div className="text-sm text-slate-400 dark:text-slate-500">کانالی ثبت نشده. «افزودن کانال» را بزنید.</div>
        )}
        {current.map((r, i) => (
          <div key={i} className="grid grid-cols-1 md:grid-cols-[1fr_1fr_1fr_auto] gap-2 items-end">
            <label className="block text-sm">
              <span className="block text-slate-600 dark:text-slate-300 mb-1">شناسه (id)</span>
              <input className="input font-mono" dir="ltr" placeholder="@channel or -100123…" value={r.id} onChange={(e) => setRow(i, { id: e.target.value })} />
            </label>
            <label className="block text-sm">
              <span className="block text-slate-600 dark:text-slate-300 mb-1">عنوان (نمایشی)</span>
              <input className="input" value={r.title} onChange={(e) => setRow(i, { title: e.target.value })} />
            </label>
            <label className="block text-sm">
              <span className="block text-slate-600 dark:text-slate-300 mb-1">لینک (اختیاری)</span>
              <input className="input" dir="ltr" placeholder="https://t.me/…" value={r.link} onChange={(e) => setRow(i, { link: e.target.value })} />
            </label>
            <button className="btn-danger px-3 py-2 text-sm" onClick={() => setRows(current.filter((_, idx) => idx !== i))}>
              حذف
            </button>
          </div>
        ))}
        <div className="flex gap-2 items-center">
          <button className="btn-secondary text-sm" onClick={() => setRows([...current, { id: '', title: '', link: '' }])}>
            ＋ افزودن کانال
          </button>
          <button className="btn-primary text-sm" disabled={saving || !dirty} onClick={save}>
            {saving ? 'در حال ذخیره…' : 'ذخیره'}
          </button>
          {note && <span className="text-sm text-slate-600 dark:text-slate-300">{note}</span>}
        </div>
      </div>
    </div>
  )
}

// ===== content template panel =====

// errText unwraps RTK-Query error payloads to a Persian message.
function errText(e: unknown): string {
  return e && typeof e === 'object' && 'data' in e
    ? ((e.data as { error?: string })?.error ?? 'خطا در ذخیره')
    : 'خطا در ذخیره'
}

// ContentTab is the whole management surface of a content bot: source and
// destination pickers (live from the MTProto sidecar), filters, brand,
// prompts and the collection ledger.
function ContentTab({ botId, onClone }: { botId: string; onClone: () => void }) {
  const { data: cfg } = useGetConfigQuery(botId)
  const { data: settings, isLoading } = useGetContentSettingsQuery(botId)
  const [saveSettings, { isLoading: saving }] = usePutContentSettingsMutation()
  const [form, setForm] = useState<ContentSettings | null>(null)
  const [note, setNote] = useState<string | null>(null)
  const [dirty, setDirty] = useState(false)

  // initialize (and re-sync after a save) from the server snapshot without an
  // effect: keep a pointer to the last consumed response
  const [lastSettings, setLastSettings] = useState<ContentSettings | null>(null)
  if (settings && settings !== lastSettings) {
    setLastSettings(settings)
    if (!dirty) setForm(settings)
  }

  if (isLoading || !settings || !form) return <div className="text-slate-500 dark:text-slate-400">در حال بارگذاری…</div>

  const set = (patch: Partial<ContentSettings>) => {
    setForm({ ...form, ...patch })
    setDirty(true)
  }

  const save = async () => {
    setNote(null)
    const { version, accounts, default_clean_prompt, default_card_prompt, ...body } = form
    void accounts; void default_clean_prompt; void default_card_prompt
    const payload: ContentSettingsPut = { ...body, version: cfg?.version ?? version }
    try {
      await saveSettings({ botId, body: payload }).unwrap()
      setNote('ذخیره شد. تغییرات بلافاصله اعمال می‌شود.')
      setDirty(false)
      if (form.account_id !== settings.account_id || form.folder !== settings.folder) {
        // switches apply on the next collector tick (۱–۲ دقیقه)
        setNote('ذخیره شد. از پُلِ بعدی، از مبدأ جدید خوانده می‌شود.')
      }
    } catch (e) {
      setNote(errText(e))
    }
  }

  const resetPrompts = () => set({ clean_prompt: '', card_prompt: '' })

  return (
    <div className="space-y-6" data-testid="content-tab">
      <LedgerStats botId={botId} />

      {/* ===== source & destination ===== */}
      <div className="card p-4 space-y-4" data-testid="content-form">
        <div className="font-semibold">🌍 مبدأ و مقصد جمع‌آوری</div>
        <p className="text-xs text-slate-500 dark:text-slate-400">
          فولدر از حساب انتخابی زنده خوانده می‌شود؛ اگر اکانت فولدری ندارد، کانال‌ها را ابتدا به یک پوشه در تلگرام اضافه کنید.
          پس از ذخیره، تغییرات از پُلِ بعدی (هر {toFaDigits(form.poll_minutes)} دقیقه) اعمال می‌شود.
        </p>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <label className="block text-sm">
            <span className="block text-slate-600 dark:text-slate-300 mb-1">اکانت تلگرام (منبع)</span>
            <select className="input" value={form.account_id}
              onChange={(e) => set({ account_id: e.target.value, folder: '', channel: '' })}>
              <option value="">— انتخاب کنید —</option>
              {settings.accounts.map((a) => (
                <option key={a.id} value={a.sidecar_account_id}>
                  {a.label || a.phone}{a.username ? ` (@${a.username})` : ''}
                </option>
              ))}
            </select>
          </label>
          <label className="block text-sm">
            <span className="block text-slate-600 dark:text-slate-300 mb-1">پوشهٔ منبع (فولدر)</span>
            <FolderSelect botId={botId} account={form.account_id} value={form.folder}
              onChange={(v) => set({ folder: v })} />
          </label>
          <label className="block text-sm">
            <span className="block text-slate-600 dark:text-slate-300 mb-1">کانال مقصد (id)</span>
            <ChannelSelect botId={botId} account={form.account_id} value={form.channel}
              onChange={(v) => set({ channel: v })} />
          </label>
          <label className="block text-sm md:col-span-3">
            <span className="block text-slate-600 dark:text-slate-300 mb-1">نحوهٔ انتشار (چه کسی کارت را می‌فرستد)</span>
            <select className="input" value={form.publish_via}
              onChange={(e) => set({ publish_via: e.target.value as 'bot' | 'sidecar' })}>
              <option value="bot">بات (اکانت ربات؛ باید ادمین کانال مقصد باشد)</option>
              <option value="sidecar">اکانت تلگرام منبع (مانند sadcontent — بدون نیاز به ادمین‌کردن بات)</option>
            </select>
            <span className="block text-xs text-slate-400 dark:text-slate-500 mt-1">
              «اکانت تلگرام» پست‌ها را از خودِ اکانت زندهٔ انتخاب‌شدهٔ بالا (مثلاً Dastan) می‌فرستد.
            </span>
          </label>
        </div>
        <StringListEditor
          title="مستثنی‌ها (کانال‌هایی که جمع نمی‌شوند)" rows={form.exclude}
          placeholder="chat_id عددی یا @username"
          onChange={(exclude) => set({ exclude })}
        />
        <StringListEditor
          title="کلیدواژه‌ها (خالی = همهٔ پیام‌ها؛ اگر پُر شود فقط پیام‌های حاوی اینها)" rows={form.keywords}
          placeholder="کلیدواژه"
          onChange={(keywords) => set({ keywords })}
        />
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <LabeledInt label="فاصلهٔ بررسی (دقیقه)" value={form.poll_minutes} min={1} max={1440}
            onChange={(poll_minutes) => set({ poll_minutes })} />
          <LabeledInt label="سقف پیام هر منبع (بک‌فیل اولیه)" value={form.backfill_limit} min={1} max={100}
            onChange={(backfill_limit) => set({ backfill_limit })} />
          <LabeledInt label="حداقل طول متن (کاراکتر)" value={form.min_length} min={0} max={4096}
            onChange={(min_length) => set({ min_length })} />
        </div>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <LabeledBool label="رد پیام‌های فورواردشده" value={form.skip_forwarded}
            onChange={(skip_forwarded) => set({ skip_forwarded })} />
          <LabeledBool label="رد پاسخ‌ها (ریپلای)" value={form.skip_replies}
            onChange={(skip_replies) => set({ skip_replies })} />
          <LabeledBool label="فقط متن (بدون کارت/عکس)" value={form.text_only}
            onChange={(text_only) => set({ text_only })} />
        </div>
        {!form.text_only && (
          <LabeledBool label="خلاصهٔ AI داخل کارت" value={form.render_card_summary}
            onChange={(render_card_summary) => set({ render_card_summary })} />
        )}
      </div>

      {/* ===== brand (cards only) ===== */}
      {!form.text_only && (
      <div className="card p-4 space-y-4">
        <div className="font-semibold">🎨 هویت برند کارت‌ها</div>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <TextField label="نام لاتین (روی کارت)" value={form.brand_name_en} onChange={(brand_name_en) => set({ brand_name_en })} mono />
          <TextField label="نام فارسی" value={form.brand_name_fa} onChange={(brand_name_fa) => set({ brand_name_fa })} />
          <TextField label="هندل" value={form.brand_handle} onChange={(brand_handle) => set({ brand_handle })} mono />
        </div>
        <div className="grid grid-cols-2 md:grid-cols-5 gap-3 items-end">
          <ColorField label="accent" value={form.brand_accent} onChange={(brand_accent) => set({ brand_accent })} />
          <ColorField label="glow A" value={form.brand_glow_a} onChange={(brand_glow_a) => set({ brand_glow_a })} />
          <ColorField label="glow B" value={form.brand_glow_b} onChange={(brand_glow_b) => set({ brand_glow_b })} />
          <ColorField label="glow C" value={form.brand_glow_c} onChange={(brand_glow_c) => set({ brand_glow_c })} />
          <div className="flex items-center gap-2">
            <div className="w-8 h-8 rounded-lg" style={{ background: form.brand_glow_a }} />
            <div className="w-8 h-8 rounded-lg" style={{ background: form.brand_glow_b }} />
            <div className="w-8 h-8 rounded-lg" style={{ background: form.brand_glow_c }} />
            <div className="w-8 h-8 rounded-lg border" style={{ background: form.brand_accent, borderColor: form.brand_accent }} />
          </div>
        </div>
        <div className="rounded-xl p-4" style={{ background: form.brand_glow_a }}>
          <span className="font-bold" style={{ color: form.brand_accent }}>{form.brand_name_en || 'brand'}</span>
          <span className="text-sm text-white/80"> — {form.brand_name_fa || 'برند'}</span>
        </div>
      </div>
      )}

      {/* ===== prompts ===== */}
      <div className="card p-4 space-y-3">
        <div className="flex items-center justify-between">
          <div className="font-semibold">🧠 پرامپت‌های AI</div>
          <button type="button" className="btn-secondary text-sm" onClick={resetPrompts}
            disabled={!form.clean_prompt && !form.card_prompt}>
            بازگردانی پیش‌فرض‌ها
          </button>
        </div>
        <p className="text-xs text-slate-500 dark:text-slate-400">
          خالی = پیش‌فرض بات (همان پرامپت پاک‌سازی/خلاصه‌سازی).
          نسخهٔ فعلی را نگه دارید: خالی‌گذاشتن پیش‌فرض‌ها را جایگزین می‌کند.
        </p>
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
          <label className="block text-sm">
            <span className="block text-slate-600 dark:text-slate-300 mb-1">پاک‌سازی خبر (CLEAN)</span>
            <textarea className="input font-mono text-xs h-64" dir="ltr" value={form.clean_prompt}
              placeholder={settings.default_clean_prompt.slice(0, 120) + '…'}
              onChange={(e) => set({ clean_prompt: e.target.value })} />
          </label>
          {!form.text_only && (
          <label className="block text-sm">
            <span className="block text-slate-600 dark:text-slate-300 mb-1">خلاصهٔ کارت (CARD)</span>
            <textarea className="input font-mono text-xs h-64" dir="ltr" value={form.card_prompt}
              placeholder={settings.default_card_prompt.slice(0, 120) + '…'}
              onChange={(e) => set({ card_prompt: e.target.value })} />
          </label>
          )}
        </div>
      </div>

      <div className="flex items-center gap-3">
        <button className="btn-primary" disabled={saving || !dirty} onClick={save} data-testid="content-save">
          {saving ? 'در حال ذخیره…' : 'ذخیرهٔ تنظیمات'}
        </button>
        <button className="btn-secondary" onClick={onClone}>⧉ کلون این بات</button>
        {note && <span className="text-sm text-slate-600 dark:text-slate-300">{note}</span>}
      </div>

      <ContentLedger botId={botId} />
    </div>
  )
}

// FolderSelect lists the account's Telegram folders (live).
function FolderSelect({ botId, account, value, onChange }: {
  botId: string; account: string; value: string; onChange: (v: string) => void
}) {
  const { data, isFetching, isError } = useGetContentFoldersQuery(
    { botId, account },
    { skip: !account }
  )
  if (!account) return <select className="input" value="" disabled><option value="">اول اکانت را بگذارید</option></select>
  if (isError) return <select className="input" value={value} disabled><option value="">خطا در خواندن فولدرها</option></select>
  const folders = data?.folders ?? []
  return (
    <select className="input" value={value} onChange={(e) => onChange(e.target.value)} disabled={isFetching}>
      <option value="">— انتخاب کنید —</option>
      {folders.map((f) => (
        <option key={f.name} value={f.name}>{f.name} ({toFaDigits(f.peers.length)})</option>
      ))}
      {value && !folders.some((f) => f.name === value) && <option value={value}>{value}</option>}
      {isFetching && <option value="">در حال خواندن…</option>}
    </select>
  )
}

// ChannelSelect lists channels/groups of the account for the destination.
function ChannelSelect({ botId, account, value, onChange }: {
  botId: string; account: string; value: string; onChange: (v: string) => void
}) {
  const { data, isFetching, isError } = useGetContentChatsQuery({ botId, account }, { skip: !account })
  const [manual, setManual] = useState(!account)
  if (!account) return <select className="input" value="" disabled><option value="">اول اکانت را بگذارید</option></select>
  const chats = isError ? [] : (data?.chats ?? [])
  return (
    <div className="space-y-1">
      {manual ? (
        <input className="input font-mono" dir="ltr" placeholder="-1001234567890" value={value}
          onChange={(e) => onChange(e.target.value)} />
      ) : (
        <select className="input" value={value} onChange={(e) => {
          if (e.target.value === '__manual__') { setManual(true); return }
          onChange(e.target.value)
        }} disabled={isFetching}>
          <option value="">— انتخاب کنید —</option>
          {chats.map((c) => (
            <option key={c.chat_id} value={c.chat_id}>{c.title || c.chat_id}</option>
          ))}
          {value && !chats.some((c) => c.chat_id === value) && <option value={value}>{value}</option>}
          <option value="__manual__">… ورود دستی chat_id</option>
        </select>
      )}
      {manual && (
        <button type="button" className="text-xs text-brand-600 hover:underline" onClick={() => setManual(false)}>
          ← انتخاب از لیست
        </button>
      )}
    </div>
  )
}

function TextField({ label, value, onChange, mono }: { label: string; value: string; onChange: (v: string) => void; mono?: boolean }) {
  return (
    <label className="block text-sm">
      <span className="block text-slate-600 dark:text-slate-300 mb-1">{label}</span>
      <input className={`input ${mono ? 'font-mono' : ''}`} dir={mono ? 'ltr' : undefined}
        value={value} onChange={(e) => onChange(e.target.value)} />
    </label>
  )
}

function ColorField({ label, value, onChange }: { label: string; value: string; onChange: (v: string) => void }) {
  const hex = valueToHex(value)
  return (
    <label className="block text-sm" dir="ltr">
      <span className="block text-slate-600 dark:text-slate-300 mb-1">{label}</span>
      <div className="flex items-center gap-2">
        <input type="color" className="w-9 h-9 rounded border-0 bg-transparent p-0" value={hex}
          onChange={(e) => onChange(e.target.value)} aria-label={label} />
        <input className="input font-mono text-xs" dir="ltr" value={value}
          onChange={(e) => onChange(e.target.value)} />
      </div>
    </label>
  )
}

// valueToHex extracts the hex core of an rgba()/hex color for the color input.
function valueToHex(v: string): string {
  const s = (v || '').trim()
  if (/^#[0-9a-fA-F]{6}$/.test(s)) return s
  const m = s.match(/(\d+)\s*,\s*(\d+)\s*,\s*(\d+)/)
  const rgb = m ? [m[1], m[2], m[3]] : ['255', '160', '112']
  return '#' + rgb.map((n) => Number(n).toString(16).padStart(2, '0')).join('')
}

function LabeledInt({ label, value, min, max, onChange }: {
  label: string; value: number; min: number; max: number; onChange: (n: number) => void
}) {
  return (
    <label className="block text-sm">
      <span className="block text-slate-600 dark:text-slate-300 mb-1">{label}</span>
      <input className="input" type="number" min={min} max={max} value={value}
        onChange={(e) => onChange(Number(e.target.value))} />
      <span className="block text-xs text-slate-400 dark:text-slate-500 mt-1" dir="ltr">
        {toFaDigits(min)} … {toFaDigits(max)}
      </span>
    </label>
  )
}

function LabeledBool({ label, value, onChange }: { label: string; value: boolean; onChange: (b: boolean) => void }) {
  return (
    <label className="block text-sm">
      <span className="block text-slate-600 dark:text-slate-300 mb-1">{label}</span>
      <select className="input" value={value ? 1 : 0} onChange={(e) => onChange(Number(e.target.value) === 1)}>
        <option value={1}>فعال</option>
        <option value={0}>غیرفعال</option>
      </select>
    </label>
  )
}

// StringListEditor manages a string-array price key as add/remove rows.
function StringListEditor({ title, rows, placeholder, onChange }: {
  title: string; rows: string[]; placeholder: string; onChange: (rows: string[]) => void
}) {
  const [draft, setDraft] = useState('')
  const add = () => {
    const v = draft.trim()
    if (!v || rows.includes(v)) return
    onChange([...rows, v])
    setDraft('')
  }
  return (
    <div className="space-y-2">
      <div className="text-sm font-medium text-slate-600 dark:text-slate-300">{title}</div>
      {rows.length === 0 && <div className="text-xs text-slate-400">خالی</div>}
      <div className="flex flex-wrap gap-2">
        {rows.map((r, i) => (
          <span key={`${r}-${i}`} className="text-xs bg-slate-100 dark:bg-slate-800 rounded-full px-3 py-1 font-mono flex items-center gap-1.5" dir="ltr">
            {r}
            <button type="button" className="text-red-500 font-bold leading-none" aria-label="حذف"
              onClick={() => onChange(rows.filter((_, idx) => idx !== i))}>×</button>
          </span>
        ))}
      </div>
      <div className="flex gap-2 max-w-md">
        <input className="input text-sm" dir="ltr" placeholder={placeholder} value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); add() } }} />
        <button type="button" className="btn-secondary text-sm" onClick={add}>افزودن</button>
      </div>
    </div>
  )
}

// LedgerStats is the stat-box row over content_posts counts.
function LedgerStats({ botId }: { botId: string }) {
  const { data, isFetching } = useGetContentPostsQuery({ botId, page: 1, size: 1 })
  const c = data?.counts ?? {}
  const box = (label: string, n: number, cls = '') => (
    <div key={label} className={`card p-3 text-center ${cls}`}>
      <div className="text-xl font-bold">{toFaDigits(n)}</div>
      <div className="text-xs text-slate-500 dark:text-slate-400">{label}</div>
    </div>
  )
  return (
    <div className="grid grid-cols-2 md:grid-cols-5 gap-3" data-testid="content-stats" style={{ opacity: isFetching ? 0.7 : 1 }}>
      {box('منتشرشده', c.published ?? 0)}
      {box('بدون محتوا', c.no_content ?? 0)}
      {box('ناموفق', c.failed ?? 0)}
      {box('فقط متن (رندر ناموفق)', c.render_failed ?? 0)}
      {box('۲۴ ساعت اخیر', data?.last_24h ?? 0)}
    </div>
  )
}

// ContentLedger is the paged, filtered table of collected posts.
function ContentLedger({ botId }: { botId: string }) {
  const [page, setPage] = useState(1)
  const [status, setStatus] = useState('')
  const [q, setQ] = useState('')
  const [qDraft, setQDraft] = useState('')
  const size = 20
  const { data, isLoading, isFetching } = useGetContentPostsQuery(
    { botId, page, size, status, q },
    { pollingInterval: 15000 } // keep the ledger fresh
  )

  const statuses: Array<[string, string]> = [
    ['', 'همه'], ['published', 'منتشرشده'], ['no_content', 'بدون محتوا'],
    ['failed', 'ناموفق'], ['render_failed', 'فقط متن'],
  ]
  const badge: Record<string, string> = {
    published: 'bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-400',
    no_content: 'bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-300',
    failed: 'bg-red-100 text-red-700 dark:bg-red-900 dark:text-red-300',
    render_failed: 'bg-amber-100 text-amber-700 dark:bg-amber-900 dark:text-amber-400',
  }
  const statusLabel: Record<string, string> = {
    published: 'منتشرشده', no_content: 'بدون محتوا', failed: 'ناموفق', render_failed: 'فقط متن',
  }
  const pages = Math.max(1, Math.ceil((data?.total ?? 0) / size))

  return (
    <div className="space-y-3" data-testid="content-ledger">
      <div className="flex items-center justify-between flex-wrap gap-2">
        <div className="font-semibold">📋 رخدادهای جمع‌آوری ({toFaDigits(data?.total ?? 0)})</div>
        <form className="flex gap-2" onSubmit={(e) => { e.preventDefault(); setPage(1); setQ(qDraft) }}>
          <input className="input text-sm" placeholder="جست‌وجو در متن…" value={qDraft}
            onChange={(e) => setQDraft(e.target.value)} />
          <button className="btn-secondary text-sm" type="submit">جست‌وجو</button>
        </form>
      </div>
      <div className="flex gap-2 flex-wrap">
        {statuses.map(([v, label]) => (
          <button key={v || 'all'} onClick={() => { setStatus(v); setPage(1) }}
            className={`text-xs rounded-full px-3 py-1 border ${
              status === v
                ? 'bg-brand-600 text-white border-brand-600'
                : 'border-slate-300 dark:border-slate-600 text-slate-600 dark:text-slate-300'
            }`}>
            {label}
          </button>
        ))}
      </div>

      {isLoading ? (
        <div className="text-slate-400">در حال بارگذاری…</div>
      ) : (
        <div className="card overflow-x-auto" style={{ opacity: isFetching ? 0.7 : 1 }}>
          <table className="w-full text-sm">
            <thead className="bg-slate-50 dark:bg-slate-900 text-slate-500 dark:text-slate-400">
              <tr>
                <th className="text-start px-4 py-2 font-medium">زمان</th>
                <th className="text-start px-4 py-2 font-medium">منبع</th>
                <th className="text-start px-4 py-2 font-medium">وضعیت</th>
                <th className="text-start px-4 py-2 font-medium">متن</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
              {data?.posts.map((p) => (
                <tr key={p.id} className="hover:bg-slate-50 dark:hover:bg-slate-800 align-top">
                  <td className="px-4 py-2 text-xs text-slate-500 whitespace-nowrap">{formatDateFa(p.created_at)}</td>
                  <td className="px-4 py-2">
                    {p.source_link ? (
                      <a href={p.source_link} target="_blank" rel="noreferrer"
                        className="text-brand-600 dark:text-brand-500 hover:underline" dir="ltr">
                        {p.source_title || p.source_chat_id}
                      </a>
                    ) : (p.source_title || String(p.source_chat_id))}
                    <div className="text-xs text-slate-400" dir="ltr">msg {toFaDigits(p.message_id)}</div>
                  </td>
                  <td className="px-4 py-2">
                    <span className={`text-xs rounded-full px-2 py-0.5 ${badge[p.status] ?? ''}`}
                      title={p.error || undefined}>
                      {statusLabel[p.status] ?? p.status}
                    </span>
                    {p.error && (
                      <div className="text-xs text-red-500 mt-1 max-w-56 truncate" title={p.error}>{p.error}</div>
                    )}
                  </td>
                  <td className="px-4 py-2 max-w-lg">
                    <span dir="auto" className="line-clamp-3">
                      {p.cleaned_text || p.raw_text || '—'}
                    </span>
                  </td>
                </tr>
              ))}
              {data && data.posts.length === 0 && (
                <tr><td colSpan={4} className="px-4 py-6 text-center text-slate-400">رخدادی با این فیلتر پیدا نشد</td></tr>
              )}
            </tbody>
          </table>
        </div>
      )}

      {pages > 1 && (
        <div className="flex items-center gap-3 text-sm">
          <button className="btn-secondary text-sm" disabled={page <= 1} onClick={() => setPage(page - 1)}>قبلی</button>
          <span dir="ltr">صفحه {toFaDigits(page)} / {toFaDigits(pages)}</span>
          <button className="btn-secondary text-sm" disabled={page >= pages} onClick={() => setPage(page + 1)}>بعدی</button>
        </div>
      )}
    </div>
  )
}

// CloneModal creates a copy of the bot via BotFather and copies its config.
function CloneModal({ botId, botName, onClose, onDone }: {
  botId: string; botName: string; onClose: () => void; onDone: (newId: string, configError?: string) => void
}) {
  const { data: accountsData } = useTgListAccountsQuery()
  const accounts = accountsData?.accounts ?? []
  const [name, setName] = useState(`${botName} (کلون)`)
  const [accountId, setAccountId] = useState('')
  const [hint, setHint] = useState('')
  const [clone, { isLoading }] = useCloneBotMutation()

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    try {
      const res = await clone({
        botId,
        account_id: accountId || accounts[0]?.id || '',
        name: name.trim() || undefined,
        username_hint: hint.trim() || undefined,
      }).unwrap()
      onDone(res.bot.id, res.config_error)
    } catch (err) {
      alert(errText(err))
    }
  }

  return (
    <div className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center p-4" onClick={onClose} role="presentation">
      <div className="card p-6 w-full max-w-lg space-y-4" onClick={(e) => e.stopPropagation()} role="dialog" aria-modal="true">
        <div className="font-semibold text-lg">⧉ کلون بات محتوا</div>
        <p className="text-sm text-slate-500 dark:text-slate-400">
          بات جدید از طریق BotFather روی اکانت انتخابی ساخته و کل تنظیمات (فولدر، برند، پرامپت‌ها…) کپی می‌شود.
          کانال مقصد و اکانتِ منبع عمداً خالی می‌مانند تا خودتان تعیین کنید.
        </p>
        <form onSubmit={submit} className="space-y-3">
          <label className="block text-sm">
            <span className="block text-slate-600 dark:text-slate-300 mb-1">نام بات جدید</span>
            <input className="input" value={name} onChange={(e) => setName(e.target.value)} required />
          </label>
          <label className="block text-sm">
            <span className="block text-slate-600 dark:text-slate-300 mb-1">ساخت روی کدام اکانت؟</span>
            <select className="input" value={accountId || accounts[0]?.id || ''}
              onChange={(e) => setAccountId(e.target.value)} required>
              {accounts.length === 0 && <option value="">اکانتی اضافه نکرده‌اید</option>}
              {accounts.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.display_name || a.label || a.phone}{a.username ? ` (@${a.username})` : ''}
                </option>
              ))}
            </select>
          </label>
          <label className="block text-sm">
            <span className="block text-slate-600 dark:text-slate-300 mb-1">پایهٔ آیدی (اختیاری)</span>
            <input className="input font-mono" dir="ltr" placeholder="MyCoolBot" value={hint}
              onChange={(e) => setHint(e.target.value)} />
          </label>
          <div className="flex gap-2 pt-2">
            <button className="btn-primary" disabled={isLoading || accounts.length === 0}>
              {isLoading ? 'در حال گفتگو با BotFather… (۳۰–۶۰ ثانیه)' : 'ساخت کلون'}
            </button>
            <button type="button" className="btn-secondary" onClick={onClose}>انصراف</button>
          </div>
        </form>
      </div>
    </div>
  )
}

function AITab({ botId }: { botId: string }) {
  const { data, isLoading } = useGetAIQuery(botId)
  const [setAI, { isLoading: toggling }] = useSetAIEnabledMutation()
  const [open, setOpen] = useState<number | null>(null)
  const [note, setNote] = useState<string | null>(null)

  const toggle = async (enabled: boolean) => {
    setNote(null)
    try {
      await setAI({ botId, enabled }).unwrap()
      setNote(enabled ? 'دستیار هوشمند فعال شد.' : 'دستیار هوشمند غیرفعال شد.')
    } catch {
      setNote('خطا در تغییر وضعیت')
    }
  }

  if (isLoading || !data) return <div className="text-slate-500 dark:text-slate-400">در حال بارگذاری…</div>

  return (
    <div className="space-y-4">
      <div className="card p-4 flex items-center justify-between gap-3">
        <div>
          <div className="font-semibold">دستیار هوشمند (AI)</div>
          <div className="text-sm text-slate-500 dark:text-slate-400">
            وقتی غیرفعال باشد، کاربران از منوی «چت با دستیار» پیام غیرفعالی می‌گیرند. گفتگوهای فعلی زیر لیست می‌شوند.
          </div>
        </div>
        <div className="flex items-center gap-2">
          <span className={`text-sm ${data.enabled ? 'text-green-600 dark:text-green-400' : 'text-slate-400 dark:text-slate-500'}`}>
            {data.enabled ? '● فعال' : '○ غیرفعال'}
          </span>
          <button className="btn-secondary text-sm" disabled={toggling} onClick={() => toggle(!data.enabled)}>
            {data.enabled ? 'غیرفعال کردن' : 'فعال کردن'}
          </button>
        </div>
      </div>
      {note && <div className="text-sm text-slate-600 dark:text-slate-300">{note}</div>}

      <div className="card p-4">
        <div className="font-semibold mb-3">گفتگوهای فعال AI ({toFaDigits(data.conversations.length)})</div>
        {data.conversations.length === 0 ? (
          <div className="text-sm text-slate-400 dark:text-slate-500">کاربری الان در صحنهٔ AI نیست.</div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-slate-500 dark:text-slate-400 border-b border-slate-200 dark:border-slate-700">
                  <th className="text-right py-2 px-2">کاربر</th>
                  <th className="text-right py-2 px-2">پیام‌ها</th>
                  <th className="text-right py-2 px-2">آخرین پیام</th>
                  <th className="text-right py-2 px-2">پیام رایگان مصرفی</th>
                  <th className="text-right py-2 px-2">آخرین فعالیت</th>
                  <th className="text-right py-2 px-2"></th>
                </tr>
              </thead>
              <tbody>
                {data.conversations.map((c) => (
                  <AITableRow key={c.user_id} botId={botId} conv={c} open={open === c.user_id} onToggle={() => setOpen(open === c.user_id ? null : c.user_id)} />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}

function AITableRow({ botId, conv, open, onToggle }: {
  botId: string
  conv: { user_id: number; telegram_id: number; username?: string; first_name?: string; messages: number; last_message?: string; ai_free_used: number; is_blocked: boolean; is_ghost: boolean; last_seen_at: string }
  open: boolean
  onToggle: () => void
}) {
  const { data: detail } = useGetAIDetailQuery({ botId, userId: conv.user_id }, { skip: !open })
  const name = conv.first_name || conv.username || `#${conv.user_id}`
  return (
    <>
      <tr className="border-b border-slate-100 dark:border-slate-800">
        <td className="py-2 px-2">
          <div>{name} {conv.is_ghost && <span className="text-xs text-slate-400 dark:text-slate-500">(جعلی)</span>}</div>
          <div className="text-xs text-slate-400 dark:text-slate-500 font-mono" dir="ltr">{conv.telegram_id}</div>
        </td>
        <td className="py-2 px-2">{toFaDigits(conv.messages)}</td>
        <td className="py-2 px-2 max-w-[280px] truncate">{conv.last_message ?? '—'}</td>
        <td className="py-2 px-2">{toFaDigits(conv.ai_free_used)}</td>
        <td className="py-2 px-2 text-slate-500 dark:text-slate-400">{formatDateFa(conv.last_seen_at)}</td>
        <td className="py-2 px-2">
          <button className="btn-ghost text-xs" onClick={onToggle}>{open ? 'بستن' : 'تاریخچه'}</button>
        </td>
      </tr>
      {open && (
        <tr className="bg-slate-50 dark:bg-slate-900">
          <td colSpan={6} className="py-3 px-4">
            {!detail ? (
              <div className="text-slate-500 dark:text-slate-400 text-sm">در حال بارگذاری تاریخچه…</div>
            ) : (
              <div className="space-y-1 text-sm">
                {(detail.history ?? []).map((m, i) => (
                  <div key={i} className={m.role === 'user' ? 'text-slate-800 dark:text-slate-100' : 'text-brand-700 dark:text-brand-500'}>
                    <span className="font-medium">{m.role === 'user' ? 'کاربر:' : 'AI:'}</span>{' '}
                    <span dir="auto">{m.content}</span>
                  </div>
                ))}
                {(detail.history ?? []).length === 0 && <div className="text-slate-400 dark:text-slate-500">تاریخچه‌ای ذخیره نشده.</div>}
              </div>
            )}
          </td>
        </tr>
      )}
    </>
  )
}

function SettingsTab({ botId }: { botId: string }) {
  const { data: cfg } = useGetConfigQuery(botId)
  const { data, isLoading } = useGetSettingsQuery(botId)
  const [putSettings, { isLoading: saving }] = usePutSettingsMutation()
  const [values, setValues] = useState<Record<string, number>>({})
  const [note, setNote] = useState<string | null>(null)

  const groupLabels: Record<string, string> = {
    bonuses: '🎁 هدیه‌ها',
    browse: '💘 مرور پروفایل',
    coldstart: '👻 کاربران جعلی',
    content: '💬 محتوا',
    growth: '📈 رشد',
    notify: '🔔 اعلان‌ها',
    payment: '💳 پرداخت',
    search: '🔍 جستجو',
  }

  if (isLoading || !data) return <div className="text-slate-500 dark:text-slate-400">در حال بارگذاری…</div>

  const val = (f: { key: string; value: number }) => values[f.key] ?? f.value

  const save = async () => {
    setNote(null)
    if (Object.keys(values).length === 0) {
      setNote('تغییری اعمال نشده')
      return
    }
    try {
      await putSettings({ botId, version: cfg?.version ?? 0, values }).unwrap()
      setValues({})
      setNote('ذخیره شد. تغییرات بلافاصله روی بات اعمال می‌شود.')
    } catch (e) {
      const msg = e && typeof e === 'object' && 'data' in e
        ? ((e.data as { error?: string })?.error ?? 'خطا در ذخیره')
        : 'خطا در ذخیره'
      setNote(msg)
    }
  }

  return (
    <div className="space-y-4">
      <div className="text-sm text-slate-500 dark:text-slate-400">
        فقط کلیدهای عددی ویرایش می‌شوند؛ کانال‌ها، پکیج‌ها و سایر تنظیمات پیشرفته دست‌نخورده می‌مانند
        (برای آن‌ها از تب «تنظیمات و منو» استفاده کنید).
      </div>
      {data.groups.map((g) => {
        const fields = data.fields.filter((f) => f.group === g)
        if (fields.length === 0) return null
        return (
          <div key={g} className="card p-4">
            <div className="font-semibold mb-3">{groupLabels[g] ?? g}</div>
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
              {fields.map((f) => (
                <label key={f.key} className="block text-sm">
                  <span className="block text-slate-600 dark:text-slate-300 mb-1">{f.label}</span>
                  {f.kind === 'bool' ? (
                    <select
                      className="input"
                      value={val(f)}
                      onChange={(e) => setValues((v) => ({ ...v, [f.key]: Number(e.target.value) }))}
                    >
                      <option value={1}>فعال</option>
                      <option value={0}>غیرفعال</option>
                    </select>
                  ) : (
                    <input
                      className="input"
                      dir="ltr"
                      type="number"
                      min={f.min}
                      max={f.max}
                      value={val(f)}
                      onChange={(e) => setValues((v) => ({ ...v, [f.key]: Number(e.target.value) }))}
                    />
                  )}
                  <span className="block text-xs text-slate-400 dark:text-slate-500 mt-1" dir="ltr">
                    {toFaDigits(f.min)} … {toFaDigits(f.max)}
                  </span>
                </label>
              ))}
            </div>
          </div>
        )
      })}
      <div className="flex items-center gap-3">
        <button className="btn-primary" disabled={saving} onClick={save}>
          ذخیرهٔ تنظیمات
        </button>
        {note && <span className="text-sm text-slate-600 dark:text-slate-300">{note}</span>}
      </div>
    </div>
  )
}

function BroadcastTab({ botId }: { botId: string }) {
  const [text, setText] = useState('')
  const [photo, setPhoto] = useState('')
  const [footer, setFooter] = useState('')
  const { data: preview } = useBroadcastPreviewQuery(botId)
  const { data: runs } = useListBroadcastsQuery(botId, { pollingInterval: 4000 })
  const [start, { isLoading }] = useStartBroadcastMutation()

  const onSend = async () => {
    if (!text.trim()) return
    const hasPhoto = photo.trim() !== ''
    const what = hasPhoto ? 'همراه با عکس' : 'متنی'
    if (!confirm(`اعلان ${what} برای ${toFaDigits(preview?.audience ?? 0)} کاربر ارسال شود؟`)) return
    await start({
      botId,
      text: text.trim(),
      ...(hasPhoto ? { photo: photo.trim(), ...(footer.trim() ? { footer: footer.trim() } : {}) } : {}),
    }).unwrap()
    setText('')
    setPhoto('')
    setFooter('')
  }

  return (
    <div className="space-y-4">
      <form
        className="card p-4 space-y-3"
        onSubmit={(e) => {
          e.preventDefault()
          void onSend()
        }}
      >
        <label className="block text-sm font-semibold">📣 اعلان همگانی</label>
        <p className="text-xs text-slate-500 dark:text-slate-400">
          مخاطبان: {toFaDigits(preview?.audience ?? 0)} کاربر (کاربران مسدودشده حذف می‌شوند)
        </p>
        <textarea
          className="input h-28"
          placeholder="متن اعلان…"
          value={text}
          onChange={(e) => setText(e.target.value)}
          required
        />
        <p className="text-xs text-slate-400 dark:text-slate-500">
          با عکس، متن به‌صورت caption زیر عکس می‌آید (حداکثر ۱۰۲۴ کاراکتر).
        </p>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
          <label className="block text-sm">
            <span className="block text-slate-600 dark:text-slate-300 mb-1">عکس (اختیاری — لینک مستقیم https یا file_id تلگرام)</span>
            <input className="input font-mono text-xs" dir="ltr" placeholder="https://…  or  AgACAgQ…" value={photo} onChange={(e) => setPhoto(e.target.value)} />
          </label>
          <label className="block text-sm">
            <span className="block text-slate-600 dark:text-slate-300 mb-1">پی‌نوشت (زیر عکس، اختیاری)</span>
            <input className="input" placeholder="مثلاً لینک ثبت‌نام" value={footer} onChange={(e) => setFooter(e.target.value)} disabled={!photo.trim()} />
          </label>
        </div>
        <button className="btn-primary" disabled={isLoading || !text.trim()}>
          {isLoading ? 'در حال شروع…' : photo.trim() ? 'ارسال اعلان با عکس' : 'ارسال اعلان'}
        </button>
      </form>

      <div className="card">
        <table className="w-full text-sm">
          <thead className="bg-slate-50 dark:bg-slate-900 text-slate-500 dark:text-slate-400">
            <tr>
              <th className="text-start px-4 py-3">متن</th>
              <th className="text-start px-4 py-3">نوع</th>
              <th className="text-start px-4 py-3">مخاطبان</th>
              <th className="text-start px-4 py-3">ارسال‌شده</th>
              <th className="text-start px-4 py-3">خطا</th>
              <th className="text-start px-4 py-3">وضعیت</th>
              <th className="text-start px-4 py-3">زمان</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
            {runs?.runs.map((r) => (
              <tr key={r.id} className="hover:bg-slate-50 dark:hover:bg-slate-800">
                <td className="px-4 py-2 max-w-md truncate" title={r.text}>{r.text}</td>
                <td className="px-4 py-2">{r.photo ? '🖼 با عکس' : 'متنی'}</td>
                <td className="px-4 py-2">{toFaDigits(r.audience)}</td>
                <td className="px-4 py-2 text-green-600 dark:text-green-400">{toFaDigits(r.sent)}</td>
                <td className="px-4 py-2 text-red-500 dark:text-red-400">{toFaDigits(r.failed)}</td>
                <td className="px-4 py-2">{r.status === 'running' ? 'در حال ارسال…' : 'تمام شد'}</td>
                <td className="px-4 py-2 text-slate-500 dark:text-slate-400 whitespace-nowrap">{formatDateFa(r.created_at)}</td>
              </tr>
            ))}
            {runs && runs.runs.length === 0 && (
              <tr><td colSpan={7} className="p-8 text-center text-slate-400 dark:text-slate-500">اعلانی ارسال نشده</td></tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function DiscountsTab({ botId }: { botId: string }) {
  const { data } = useListDiscountsQuery(botId)
  const [create, { isLoading }] = useCreateDiscountMutation()
  const [remove] = useDeleteDiscountMutation()
  const [code, setCode] = useState('')
  const [kind, setKind] = useState('percent')
  const [value, setValue] = useState('')
  const [maxUses, setMaxUses] = useState('')
  const [expires, setExpires] = useState('')

  const onCreate = async (e: FormEvent) => {
    e.preventDefault()
    await create({
      botId,
      code: code.trim(),
      kind,
      value: Number(value),
      max_uses: maxUses ? Number(maxUses) : 0,
      expires_at: expires ? new Date(expires).toISOString() : undefined,
    }).unwrap()
    setCode('')
    setValue('')
    setMaxUses('')
    setExpires('')
  }

  const kindLabels: Record<string, string> = {
    percent: 'درصد تخفیف',
    flat_toman: 'تومان تخفیف',
    coin_bonus: 'هدیه سکه',
  }

  return (
    <div className="space-y-4">
      <form onSubmit={onCreate} className="card p-4 flex flex-wrap items-end gap-3">
        <div>
          <label className="block text-xs text-slate-500 dark:text-slate-400 mb-1">کد</label>
          <input className="input w-40" dir="ltr" placeholder="OFF50" value={code} onChange={(e) => setCode(e.target.value)} required />
        </div>
        <div>
          <label className="block text-xs text-slate-500 dark:text-slate-400 mb-1">نوع</label>
          <select className="input w-44" value={kind} onChange={(e) => setKind(e.target.value)}>
            {Object.entries(kindLabels).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
          </select>
        </div>
        <div>
          <label className="block text-xs text-slate-500 dark:text-slate-400 mb-1">{kind === 'percent' ? 'درصد' : kind === 'flat_toman' ? 'تومان' : 'سکه'}</label>
          <input className="input w-28" dir="ltr" type="number" min={1} value={value} onChange={(e) => setValue(e.target.value)} required />
        </div>
        <div>
          <label className="block text-xs text-slate-500 dark:text-slate-400 mb-1">سقف استفاده (۰ = نامحدود)</label>
          <input className="input w-36" dir="ltr" type="number" min={0} value={maxUses} onChange={(e) => setMaxUses(e.target.value)} />
        </div>
        <div>
          <label className="block text-xs text-slate-500 dark:text-slate-400 mb-1">انقضا (اختیاری)</label>
          <input className="input w-48" type="datetime-local" value={expires} onChange={(e) => setExpires(e.target.value)} />
        </div>
        <button className="btn-primary" disabled={isLoading}>ساخت کد</button>
      </form>

      <div className="card">
        <table className="w-full text-sm">
          <thead className="bg-slate-50 dark:bg-slate-900 text-slate-500 dark:text-slate-400">
            <tr>
              <th className="text-start px-4 py-3">کد</th>
              <th className="text-start px-4 py-3">نوع</th>
              <th className="text-start px-4 py-3">مقدار</th>
              <th className="text-start px-4 py-3">استفاده</th>
              <th className="text-start px-4 py-3">انقضا</th>
              <th className="text-start px-4 py-3"></th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
            {data?.discounts.map((d) => (
              <tr key={d.id} className="hover:bg-slate-50 dark:hover:bg-slate-800">
                <td className="px-4 py-2 font-mono" dir="ltr">{d.code}</td>
                <td className="px-4 py-2">{kindLabels[d.kind] ?? d.kind}</td>
                <td className="px-4 py-2">
                  {toFaDigits(d.value)}{d.kind === 'percent' ? '٪' : d.kind === 'flat_toman' ? ' تومان' : ' سکه'}
                </td>
                <td className="px-4 py-2">
                  {toFaDigits(d.used_count)}{d.max_uses > 0 ? ` از ${toFaDigits(d.max_uses)}` : ''}
                </td>
                <td className="px-4 py-2 text-slate-500 dark:text-slate-400">{d.expires_at ? formatDateFa(d.expires_at) : '—'}</td>
                <td className="px-4 py-2">
                  <button className="btn-danger px-3 py-1 text-xs" onClick={() => remove({ botId, id: d.id })}>
                    حذف
                  </button>
                </td>
              </tr>
            ))}
            {data && data.discounts.length === 0 && (
              <tr><td colSpan={6} className="p-8 text-center text-slate-400 dark:text-slate-500">کد تخفیفی ساخته نشده</td></tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function ActivityTab({ botId }: { botId: string }) {
  const [kind, setKind] = useState('')
  const [page, setPage] = useState(1)
  const { data } = useListEventsQuery({ botId, kind, page })
  const kindLabels: Record<string, string> = {
    inbound_message: 'پیام ورودی',
    outbound_message: 'پیام خروجی',
    callback: 'تپ دکمه',
    command: 'دستور',
    user_start: 'شروع کاربر',
    state_change: 'تغییر وضعیت',
  }
  return (
    <div className="card">
      <div className="p-4 border-b border-slate-200 dark:border-slate-700 flex gap-3">
        <select className="input max-w-xs" value={kind} onChange={(e) => { setKind(e.target.value); setPage(1) }}>
          <option value="">همهٔ رویدادها</option>
          {Object.entries(kindLabels).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
        </select>
      </div>
      <table className="w-full text-sm">
        <thead className="bg-slate-50 dark:bg-slate-900 text-slate-500 dark:text-slate-400">
          <tr>
            <th className="text-start px-4 py-3">نوع</th>
            <th className="text-start px-4 py-3">کاربر</th>
            <th className="text-start px-4 py-3">متن</th>
            <th className="text-start px-4 py-3">زمان</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
          {data?.events.map((e) => (
            <tr key={e.id} className="hover:bg-slate-50 dark:hover:bg-slate-800">
              <td className="px-4 py-2">{kindLabels[e.kind] ?? e.kind}</td>
              <td className="px-4 py-2 font-mono text-xs">{e.username ? `@${e.username}` : e.first_name ?? toFaDigits(e.user_id)}</td>
              <td className="px-4 py-2 max-w-md truncate" title={e.text}>{e.text ?? '—'}</td>
              <td className="px-4 py-2 text-slate-500 dark:text-slate-400 whitespace-nowrap">{formatDateFa(e.created_at)}</td>
            </tr>
          ))}
          {data && data.events.length === 0 && (
            <tr><td colSpan={4} className="p-8 text-center text-slate-400 dark:text-slate-500">رویدادی ثبت نشده</td></tr>
          )}
        </tbody>
      </table>
      <Paginator page={page} total={data?.total ?? 0} onPage={setPage} />
    </div>
  )
}

function PaymentsTab({ botId }: { botId: string }) {
  const [page, setPage] = useState(1)
  const { data } = useListPaymentsQuery({ botId, page })
  const [createPayment, { isLoading: saving }] = useCreatePaymentMutation()
  const [amount, setAmount] = useState('')
  const [note, setNote] = useState('')
  const [userId, setUserId] = useState('')

  const onRecord = async (e: FormEvent) => {
    e.preventDefault()
    await createPayment({
      botId,
      amount_rials: Math.round(Number(amount.replace(/,/g, '')) * 10), // Toman input → Rial storage
      note: note || undefined,
      user_id: userId ? Number(userId) : undefined,
    }).unwrap()
    setAmount('')
    setNote('')
    setUserId('')
  }

  return (
    <div className="space-y-4">
      <form onSubmit={onRecord} className="card p-4 flex flex-wrap items-end gap-3">
        <div>
          <label className="block text-xs text-slate-500 dark:text-slate-400 mb-1">مبلغ (تومان)</label>
          <input className="input w-40" dir="ltr" placeholder="50000" value={amount} onChange={(e) => setAmount(e.target.value)} required />
        </div>
        <div>
          <label className="block text-xs text-slate-500 dark:text-slate-400 mb-1">شناسهٔ کاربر (اختیاری)</label>
          <input className="input w-40" dir="ltr" placeholder="user id" value={userId} onChange={(e) => setUserId(e.target.value)} />
        </div>
        <div className="flex-1 min-w-48">
          <label className="block text-xs text-slate-500 dark:text-slate-400 mb-1">یادداشت</label>
          <input className="input" value={note} onChange={(e) => setNote(e.target.value)} placeholder="مثلاً: خرید دستی بسته ۱۰ سکه" />
        </div>
        <button className="btn-primary" disabled={saving}>ثبت پرداخت</button>
      </form>

      <div className="card">
        <table className="w-full text-sm">
          <thead className="bg-slate-50 dark:bg-slate-900 text-slate-500 dark:text-slate-400">
            <tr>
              <th className="text-start px-4 py-3">کاربر</th>
              <th className="text-start px-4 py-3">مبلغ</th>
              <th className="text-start px-4 py-3">نوع</th>
              <th className="text-start px-4 py-3">وضعیت</th>
              <th className="text-start px-4 py-3">یادداشت</th>
              <th className="text-start px-4 py-3">زمان</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
            {data?.payments.map((p) => (
              <tr key={p.id} className="hover:bg-slate-50 dark:hover:bg-slate-800">
                <td className="px-4 py-2 font-mono text-xs">{p.username ? `@${p.username}` : p.first_name ?? '—'}</td>
                <td className="px-4 py-2 font-medium">{formatRial(p.amount_rials)}</td>
                <td className="px-4 py-2">{p.kind === 'manual' ? 'دستی' : p.kind === 'coin_package' ? 'بسته سکه' : 'اشتراک'}</td>
                <td className="px-4 py-2">
                  <span className={`inline-block px-2 py-0.5 rounded-full text-xs font-medium ${
                    p.status === 'verified' ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
                    : p.status === 'pending' ? 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400'
                    : p.status === 'refunded' ? 'bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-400'
                    : 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'
                  }`}>
                    {p.status === 'verified' ? 'موفق' : p.status === 'pending' ? 'در انتظار' : p.status === 'refunded' ? 'بازگشت‌شده' : 'ناموفق'}
                  </span>
                </td>
                <td className="px-4 py-2 text-slate-500 dark:text-slate-400">{p.note ?? '—'}</td>
                <td className="px-4 py-2 text-slate-500 dark:text-slate-400 whitespace-nowrap">{formatDateFa(p.created_at)}</td>
              </tr>
            ))}
            {data && data.payments.length === 0 && (
              <tr><td colSpan={6} className="p-8 text-center text-slate-400 dark:text-slate-500">پرداختی ثبت نشده</td></tr>
            )}
          </tbody>
        </table>
        <Paginator page={page} total={data?.total ?? 0} onPage={setPage} />
      </div>
    </div>
  )
}

function ConfigTab({ botId }: { botId: string }) {
  const { data: cfg, isLoading } = useGetConfigQuery(botId)
  const [putConfig, { isLoading: saving, error }] = usePutConfigMutation()
  const [texts, setTexts] = useState<string | null>(null)
  const [menus, setMenus] = useState<string | null>(null)
  const [prices, setPrices] = useState<string | null>(null)
  const [saved, setSaved] = useState(false)

  if (isLoading || !cfg) return <div className="text-slate-500 dark:text-slate-400">در حال بارگذاری…</div>

  const textsVal = texts ?? prettyJSON(cfg.texts)
  const menusVal = menus ?? prettyJSON(cfg.menus)
  const pricesVal = prices ?? prettyJSON(cfg.prices)
  const errMsg = error && typeof error === 'object' && 'data' in error
    ? ((error.data as { error?: string })?.error ?? 'خطا در ذخیره')
    : null

  const onSave = async (e: FormEvent) => {
    e.preventDefault()
    setSaved(false)
    try {
      JSON.parse(textsVal)
      JSON.parse(menusVal)
      JSON.parse(pricesVal)
    } catch {
      alert('JSON نامعتبر است')
      return
    }
    await putConfig({ id: botId, version: cfg.version, texts: textsVal, menus: menusVal, prices: pricesVal }).unwrap()
    setSaved(true)
  }

  return (
    <form onSubmit={onSave} className="space-y-4">
      <p className="text-sm text-slate-500 dark:text-slate-400">
        نسخهٔ فعلی: {toFaDigits(cfg.version)} — ذخیره با کنترل نسخه (نسخه‌های همزمان برخورد نمی‌کنند).
      </p>
      <MenuEditor value={menusVal} onChange={setMenus} />
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        <div className="card p-4">
          <label className="block text-sm font-semibold mb-2">متن‌ها (texts)</label>
          <textarea className="input font-mono text-xs h-96" dir="ltr" value={textsVal} onChange={(e) => setTexts(e.target.value)} />
        </div>
        <div className="card p-4">
          <label className="block text-sm font-semibold mb-2">منوها (menus — صحنه‌ها و دکمه‌ها)</label>
          <textarea className="input font-mono text-xs h-96" dir="ltr" value={menusVal} onChange={(e) => setMenus(e.target.value)} />
        </div>
        <div className="card p-4">
          <label className="block text-sm font-semibold mb-2">قیمت‌ها (prices — سکه، تخفیف‌ها، ghost و…)</label>
          <textarea className="input font-mono text-xs h-96" dir="ltr" value={pricesVal} onChange={(e) => setPrices(e.target.value)} />
        </div>
      </div>
      {errMsg && <div className="text-sm text-red-600 dark:text-red-400 bg-red-50 dark:bg-red-950 border border-red-200 dark:border-red-800 rounded-lg p-3">{errMsg}</div>}
      {saved && <div className="text-sm text-green-700 dark:text-green-400 bg-green-50 dark:bg-green-950 border border-green-200 dark:border-green-800 rounded-lg p-3">ذخیره شد. تغییرات بلافاصله اعمال می‌شود.</div>}
      <button className="btn-primary" disabled={saving}>{saving ? 'در حال ذخیره…' : 'ذخیره تنظیمات'}</button>
    </form>
  )
}

// MenuEditor offers a structured, per-button view of menus.scenes on top of
// the raw JSON: pick a scene, edit each button's label/action, add/remove.
// Plain-string buttons (reply keyboards) stay strings; editing one converts
// it to {"label","action"} form.
function MenuEditor({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  type Btn = { label: string; action: string }
  const parsed = useMemo(() => {
    try {
      const o = JSON.parse(value) as { scenes?: Record<string, { rows?: unknown[][] }> }
      return o && typeof o === 'object' ? o : null
    } catch {
      return null
    }
  }, [value])
  const sceneNames = parsed?.scenes ? Object.keys(parsed.scenes) : []
  const [sel, setSel] = useState('')
  const sceneName = sceneNames.includes(sel) ? sel : (sceneNames[0] ?? '')

  if (!parsed?.scenes || sceneNames.length === 0) return null
  const scene = parsed.scenes[sceneName] ?? { rows: [] }

  const rows: Btn[][] = (scene.rows ?? []).map((row) =>
    (Array.isArray(row) ? row : []).map((b) =>
      typeof b === 'string'
        ? { label: b, action: '' }
        : { label: String((b as { label?: unknown }).label ?? ''), action: String((b as { action?: unknown }).action ?? '') },
    ),
  )

  const commit = (nextRows: Btn[][]) => {
    const scenes = { ...parsed.scenes, [sceneName]: { ...(scene as object), rows: nextRows.map((row) => row.map((b) => (b.action.trim() ? { label: b.label, action: b.action } : b.label))) } }
    onChange(JSON.stringify({ ...parsed, scenes }, null, 2))
  }

  return (
    <div className="card p-4 space-y-3">
      <div className="flex items-center justify-between flex-wrap gap-2">
        <div>
          <div className="font-semibold">🎛 ویرایشگر دکمه‌های منو</div>
          <div className="text-sm text-slate-500 dark:text-slate-400">
            ساختاری و دکمه‌به‌دکمه؛ JSON خام پایین همیشه در دسترس است. دکمهٔ بدون action به‌صورت متن ساده ذخیره می‌شود.
          </div>
        </div>
        <select className="input max-w-xs" value={sceneName} onChange={(e) => setSel(e.target.value)}>
          {sceneNames.map((n) => <option key={n} value={n}>صحنه: {n}</option>)}
        </select>
      </div>
      {rows.map((row, ri) => (
        <div key={ri} className="border border-slate-200 dark:border-slate-700 rounded-lg p-3 space-y-2">
          <div className="flex items-center justify-between">
            <span className="text-xs text-slate-400 dark:text-slate-500">ردیف {toFaDigits(ri + 1)}</span>
            <button type="button" className="btn-ghost text-xs text-red-600 dark:text-red-400" onClick={() => commit(rows.filter((_, i) => i !== ri))}>حذف ردیف</button>
          </div>
          <div className="flex flex-wrap gap-2">
            {row.map((b, bi) => (
              <div key={bi} className="flex items-center gap-1 bg-slate-50 dark:bg-slate-900 rounded-lg p-2">
                <input
                  className="input w-44 text-sm"
                  placeholder="متن دکمه"
                  value={b.label}
                  onChange={(e) => commit(rows.map((r, i) => (i === ri ? r.map((x, j) => (j === bi ? { ...x, label: e.target.value } : x)) : r)))}
                />
                <input
                  className="input w-40 text-xs font-mono"
                  dir="ltr"
                  placeholder="action (اختیاری)"
                  value={b.action}
                  onChange={(e) => commit(rows.map((r, i) => (i === ri ? r.map((x, j) => (j === bi ? { ...x, action: e.target.value } : x)) : r)))}
                />
                <button type="button" className="btn-ghost text-xs text-red-600 dark:text-red-400" onClick={() => commit(rows.map((r, i) => (i === ri ? r.filter((_, j) => j !== bi) : r)))}>✕</button>
              </div>
            ))}
            <button
              type="button"
              className="btn-ghost text-xs"
              onClick={() => commit(rows.map((r, i) => (i === ri ? [...r, { label: '', action: '' }] : r)))}
            >
              ＋ دکمه
            </button>
          </div>
        </div>
      ))}
      <button type="button" className="btn-secondary text-sm" onClick={() => commit([...rows, [{ label: '', action: '' }]])}>
        ＋ ردیف جدید
      </button>
    </div>
  )
}

function Paginator({ page, total, onPage }: { page: number; total: number; onPage: (p: number) => void }) {
  const size = 20
  const pages = Math.max(1, Math.ceil(total / size))
  if (pages <= 1) return null
  return (
    <div className="flex items-center justify-between px-4 py-3 border-t border-slate-200 dark:border-slate-700 text-sm">
      <button className="btn-secondary py-1" disabled={page <= 1} onClick={() => onPage(page - 1)}>قبلی</button>
      <span className="text-slate-500 dark:text-slate-400">صفحهٔ {toFaDigits(page)} از {toFaDigits(pages)}</span>
      <button className="btn-secondary py-1" disabled={page >= pages} onClick={() => onPage(page + 1)}>بعدی</button>
    </div>
  )
}
