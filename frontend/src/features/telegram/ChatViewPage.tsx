import { useEffect, useMemo, useRef, useState, type FormEvent, type ReactNode } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  telegramApi,
  tgChatPhotoUrl,
  tgPhotoUrl,
  useTgGetChatsQuery,
  useTgGetMessagesQuery,
  useTgSendMessageMutation,
  type TgChat,
  type TgMessage,
} from './telegramApi'
import { useAppDispatch } from '../../app/hooks'
import { toFaDigits } from '../../lib/utils'

/** Avatar with server-photo + initial-letter fallback (blob fetch keeps the JWT header). */
function ChatAvatar({ accountId, chatId, title, className = 'w-11 h-11 text-base' }: {
  accountId: string
  chatId: string
  title: string
  className?: string
}) {
  const [url, setUrl] = useState<string | null>(null)
  useEffect(() => {
    let objectUrl: string | null = null
    let cancelled = false
    const token = localStorage.getItem('sadbot_token')
    fetch(tgChatPhotoUrl(accountId, chatId), {
      headers: token ? { Authorization: `Bearer ${token}` } : {},
    })
      .then((r) => {
        if (!r.ok) throw new Error('no photo')
        return r.blob()
      })
      .then((blob) => {
        if (cancelled) return
        objectUrl = URL.createObjectURL(blob)
        setUrl(objectUrl)
      })
      .catch(() => {
        /* keep initial fallback */
      })
    return () => {
      cancelled = true
      if (objectUrl) URL.revokeObjectURL(objectUrl)
    }
  }, [accountId, chatId])

  const initial = (title || '؟').trim().charAt(0)
  if (url) return <img src={url} alt="" className={`${className} rounded-full object-cover shrink-0`} />
  return (
    <div className={`${className} rounded-full bg-slate-300 dark:bg-slate-600 text-slate-700 dark:text-slate-200 flex items-center justify-center font-bold shrink-0`}>
      {initial}
    </div>
  )
}

const timeFmt = new Intl.DateTimeFormat('fa-IR', { hour: '2-digit', minute: '2-digit' })
const dayFmt = new Intl.DateTimeFormat('fa-IR', { dateStyle: 'full' })

function formatTime(iso?: string | null): string {
  if (!iso) return ''
  try {
    return timeFmt.format(new Date(iso))
  } catch {
    return ''
  }
}

function dayKey(iso?: string | null): string {
  if (!iso) return ''
  try {
    return new Date(iso).toDateString()
  } catch {
    return iso
  }
}

function formatDay(iso?: string | null): string {
  if (!iso) return ''
  try {
    return dayFmt.format(new Date(iso))
  } catch {
    return iso
  }
}

function PhotoMessage({ accountId, chatId, msg }: { accountId: string; chatId: string; msg: TgMessage }) {
  const [url, setUrl] = useState<string | null>(null)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    let objectUrl: string | null = null
    let cancelled = false
    const token = localStorage.getItem('sadbot_token')
    fetch(tgPhotoUrl(accountId, chatId, msg.id), {
      headers: token ? { Authorization: `Bearer ${token}` } : {},
    })
      .then((r) => {
        if (!r.ok) throw new Error('fetch failed')
        return r.blob()
      })
      .then((blob) => {
        if (cancelled) return
        objectUrl = URL.createObjectURL(blob)
        setUrl(objectUrl)
      })
      .catch(() => !cancelled && setFailed(true))
    return () => {
      cancelled = true
      if (objectUrl) URL.revokeObjectURL(objectUrl)
    }
  }, [accountId, chatId, msg.id])

  if (failed) return <div className="text-xs opacity-70">🖼 عکس بارگذاری نشد</div>
  if (!url) return <div className="text-xs opacity-70">🖼 در حال بارگذاری عکس…</div>
  return <img src={url} alt="" className="rounded-xl max-w-[280px] max-h-[320px] object-cover" loading="lazy" />
}

function Bubble({ accountId, chatId, msg, showSender }: { accountId: string; chatId: string; msg: TgMessage; showSender: boolean }) {
  const mine = msg.sender.is_out
  return (
    <div className={`flex ${mine ? 'justify-end' : 'justify-start'}`} data-mine={mine}>
      <div
        className={`max-w-[75%] rounded-2xl px-3 py-2 shadow-sm ${
          mine ? 'bg-brand-600 text-white rounded-bl-sm' : 'bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-700 rounded-br-sm'
        }`}
      >
        {showSender && !mine && msg.sender.name && (
          <div className="text-xs font-bold text-brand-600 dark:text-brand-500 mb-0.5">{msg.sender.name}</div>
        )}
        {msg.reply_to_id ? (
          <div className={`text-xs mb-1 ${mine ? 'text-white/70' : 'text-slate-400 dark:text-slate-500'}`}>↩︎ پیام #{toFaDigits(msg.reply_to_id)}</div>
        ) : null}
        {msg.media.kind === 'photo' && (
          <div className="mb-1">
            <PhotoMessage accountId={accountId} chatId={chatId} msg={msg} />
          </div>
        )}
        {msg.media.kind !== 'none' && msg.media.kind !== 'photo' && (
          <div className={`text-xs mb-1 ${mine ? 'text-white/80' : 'text-slate-500 dark:text-slate-400'}`}>
            📎 {msg.media.kind === 'sticker' ? 'استیکر' : msg.media.kind === 'voice' ? 'پیام صوتی' : msg.media.kind === 'video' ? 'ویدیو' : msg.media.kind === 'audio' ? 'صوت' : 'فایل'}
          </div>
        )}
        {msg.text && <div className="text-sm leading-6 whitespace-pre-wrap break-words">{msg.text}</div>}
        <div className={`text-[10px] mt-1 text-end ${mine ? 'text-white/60' : 'text-slate-400 dark:text-slate-500'}`}>{formatTime(msg.date)}</div>
      </div>
    </div>
  )
}

function Composer({ accountId, chatId }: { accountId: string; chatId: string }) {
  const [text, setText] = useState('')
  const [send, { isLoading }] = useTgSendMessageMutation()
  const [error, setError] = useState<string | null>(null)
  const areaRef = useRef<HTMLTextAreaElement>(null)

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault()
    const body = text.trim()
    if (!body || isLoading) return
    setError(null)
    try {
      await send({ accountId, chatId, text: body }).unwrap()
      setText('')
      if (areaRef.current) areaRef.current.style.height = 'auto'
    } catch (err) {
      const data = err && typeof err === 'object' && 'data' in err ? (err.data as { error?: string }) : null
      setError(data?.error ?? 'ارسال پیام ناموفق بود؛ دوباره تلاش کنید')
    }
  }

  return (
    <form onSubmit={onSubmit} className="border-t border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 p-3" data-testid="chat-composer">
      <div className="flex items-end gap-2">
        <textarea
          ref={areaRef}
          className="input flex-1 resize-none max-h-32 leading-6"
          dir="auto"
          rows={1}
          placeholder="پیام بنویسید… (Enter برای ارسال، Shift+Enter برای خط جدید)"
          value={text}
          onChange={(e) => {
            setText(e.target.value)
            e.target.style.height = 'auto'
            e.target.style.height = `${Math.min(e.target.scrollHeight, 128)}px`
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
              e.preventDefault()
              e.currentTarget.form?.requestSubmit()
            }
          }}
        />
        <button type="submit" className="btn-primary shrink-0" disabled={isLoading || !text.trim()}>
          {isLoading ? '…' : 'ارسال'}
        </button>
      </div>
      {error && <div className="text-xs text-red-600 dark:text-red-400 bg-red-50 dark:bg-red-950 border border-red-200 dark:border-red-800 rounded-lg p-2 mt-2">{error}</div>}
    </form>
  )
}

function MessageList({ accountId, chatId }: { accountId: string; chatId: string }) {
  const { data, isLoading } = useTgGetMessagesQuery(
    { accountId, chatId },
    { pollingInterval: 4000 },
  )
  const dispatch = useAppDispatch()
  const [loadingOlder, setLoadingOlder] = useState(false)
  const bottomRef = useRef<HTMLDivElement>(null)

  const messages = data?.messages ?? []
  const canLoadOlder = messages.length > 0 && (data?.oldest_id ?? 0) > 1

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'auto' })
  }, [messages.length, chatId])

  const loadOlder = async () => {
    if (!data?.oldest_id) return
    setLoadingOlder(true)
    try {
      await dispatch(
        telegramApi.endpoints.tgGetMessages.initiate(
          { accountId, chatId, offsetId: data.oldest_id },
          { subscribe: false, forceRefetch: true },
        ),
      ).unwrap()
    } finally {
      setLoadingOlder(false)
    }
  }

  const rows = useMemo(() => {
    const out: ReactNode[] = []
    let lastDay = ''
    for (const m of messages) {
      const k = dayKey(m.date)
      if (k && k !== lastDay) {
        lastDay = k
        out.push(
          <div key={`day-${k}`} className="flex justify-center my-2">
            <span className="text-xs bg-slate-200 dark:bg-slate-700 text-slate-600 dark:text-slate-300 rounded-full px-3 py-1">{formatDay(m.date)}</span>
          </div>,
        )
      }
      const next = messages[messages.indexOf(m) + 1]
      const showSender = !next || next.sender.id !== m.sender.id || next.sender.is_out !== m.sender.is_out
      out.push(
        <Bubble key={m.id} accountId={accountId} chatId={chatId} msg={m} showSender={showSender && !m.sender.is_out} />,
      )
    }
    return out
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [messages, accountId, chatId])

  if (isLoading) return <p className="text-slate-500 dark:text-slate-400 text-center p-8">در حال بارگذاری پیام‌ها…</p>

  return (
    <>
      <div className="flex-1 overflow-y-auto p-4 space-y-1.5 bg-slate-100 dark:bg-slate-800">
        {canLoadOlder && (
          <div className="flex justify-center">
            <button className="btn-secondary text-xs px-3 py-1" disabled={loadingOlder} onClick={loadOlder}>
              {loadingOlder ? 'در حال بارگذاری…' : 'بارگذاری پیام‌های قدیمی‌تر'}
            </button>
          </div>
        )}
        {messages.length === 0 && <p className="text-slate-400 dark:text-slate-500 text-center p-8">هنوز پیامی در این چت نیست.</p>}
        {rows}
        <div ref={bottomRef} />
      </div>
      <Composer accountId={accountId} chatId={chatId} />
    </>
  )
}

function ChatListItem({ accountId, chat, active, onClick }: { accountId: string; chat: TgChat; active: boolean; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      className={`w-full flex items-center gap-3 p-3 text-right transition-colors border-b border-slate-100 dark:border-slate-800 ${
        active ? 'bg-brand-50 dark:bg-brand-600/20' : 'hover:bg-slate-50 dark:hover:bg-slate-800'
      }`}
    >
      <ChatAvatar accountId={accountId} chatId={chat.chat_id} title={chat.title} />
      <div className="min-w-0 flex-1">
        <div className="flex items-center justify-between gap-2">
          <span className="font-medium text-sm truncate">{chat.title || '(بدون عنوان)'}</span>
          {chat.last_message?.date && (
            <span className="text-[10px] text-slate-400 dark:text-slate-500 shrink-0">{formatTime(chat.last_message.date)}</span>
          )}
        </div>
        <div className="flex items-center justify-between gap-2 mt-0.5">
          <span className="text-xs text-slate-500 dark:text-slate-400 truncate">{chat.last_message?.text || typeLabel(chat.type)}</span>
          {chat.unread > 0 && (
            <span className="text-[10px] bg-brand-600 text-white rounded-full px-1.5 py-0.5 shrink-0">
              {toFaDigits(chat.unread)}
            </span>
          )}
        </div>
      </div>
    </button>
  )
}

function typeLabel(type: string): string {
  switch (type) {
    case 'channel':
      return 'کانال'
    case 'group':
      return 'گروه'
    case 'bot':
      return 'بات'
    default:
      return 'چت خصوصی'
  }
}

export default function ChatViewPage() {
  const { accountId = '' } = useParams()
  const { data, isLoading, error } = useTgGetChatsQuery({ accountId }, { pollingInterval: 10000 })
  const [selected, setSelected] = useState<string | null>(null)
  const [search, setSearch] = useState('')

  const chats = data?.chats ?? []
  const filtered = chats.filter((c) => c.title.toLowerCase().includes(search.toLowerCase()))
  const activeChat = chats.find((c) => c.chat_id === selected) ?? null

  return (
    <div className="flex h-[calc(100vh-3rem)]">
      {/* chat list — right side in RTL */}
      <aside className="w-80 shrink-0 border-s border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 flex flex-col">
        <div className="p-3 border-b border-slate-200 dark:border-slate-700 space-y-2">
          <div className="flex items-center justify-between">
            <Link to="/telegram" className="text-sm text-brand-600 dark:text-brand-500 hover:underline">
              ← اکانت‌ها
            </Link>
          </div>
          <input
            className="input"
            placeholder="جستجو در چت‌ها…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>
        <div className="flex-1 overflow-y-auto">
          {isLoading && <p className="text-slate-500 dark:text-slate-400 text-center p-6 text-sm">در حال بارگذاری…</p>}
          {error && (
            <div className="m-3 text-sm text-red-600 dark:text-red-400 bg-red-50 dark:bg-red-950 border border-red-200 dark:border-red-800 rounded-lg p-3">
              خطا در دریافت چت‌ها. اگر اکانت از سشن افتاده، آن را حذف و دوباره اضافه کنید.
            </div>
          )}
          {!isLoading && !error && filtered.length === 0 && (
            <p className="text-slate-400 dark:text-slate-500 text-center p-6 text-sm">چتی یافت نشد.</p>
          )}
          {filtered.map((c) => (
            <ChatListItem key={c.chat_id} accountId={accountId} chat={c} active={c.chat_id === selected} onClick={() => setSelected(c.chat_id)} />
          ))}
        </div>
      </aside>

      {/* messages — left side in RTL */}
      <main className="flex-1 flex flex-col min-w-0">
        {activeChat ? (
          <>
            <header className="bg-white dark:bg-slate-900 border-b border-slate-200 dark:border-slate-700 px-4 py-3 flex items-center gap-3">
              <ChatAvatar accountId={accountId} chatId={activeChat.chat_id} title={activeChat.title} className="w-9 h-9 text-sm" />
              <div>
                <div className="font-bold text-sm">{activeChat.title}</div>
                <div className="text-xs text-slate-500 dark:text-slate-400">{typeLabel(activeChat.type)}</div>
              </div>
            </header>
            <MessageList accountId={accountId} chatId={activeChat.chat_id} />
          </>
        ) : (
          <div className="flex-1 flex items-center justify-center text-slate-400 dark:text-slate-500">
            <div className="text-center space-y-2">
              <div className="text-4xl">💬</div>
              <p>برای خواندن پیام‌ها، یک چت را از لیست انتخاب کنید.</p>
            </div>
          </div>
        )}
      </main>
    </div>
  )
}
