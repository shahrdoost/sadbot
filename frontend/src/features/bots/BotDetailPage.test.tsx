import { configureStore } from '@reduxjs/toolkit'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Provider } from 'react-redux'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { jsonResponse, stubFetch } from '../../test/stubFetch'
import { sadbotApi } from './botsApi'
import BotDetailPage from './BotDetailPage'

function renderDetail(path = '/bots/bot1') {
  const store = configureStore({
    reducer: { [sadbotApi.reducerPath]: sadbotApi.reducer },
    middleware: (getDefault) => getDefault().concat(sadbotApi.middleware),
  })
  render(
    <Provider store={store}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/bots/:id" element={<BotDetailPage />} />
        </Routes>
      </MemoryRouter>
    </Provider>,
  )
}

const baseBot = {
  bot: {
    id: 'bot1',
    name: 'بات تست',
    template_key: 'dating',
    status: 'running',
    created_at: '2026-10-14T10:00:00Z',
  },
  running: true,
}

const baseStats = {
  total_users: 0,
  new_users_30d: 0,
  revenue_30d_rials: 0,
  active_subscriptions: 0,
  events_24h: 0,
}

// Default tab is "users"; the page also fetches stats on mount — both must
// return well-formed shapes or the render crashes before tabs are reachable.
function stubBase(url: string): Response | null {
  if (url.includes('/stats')) return jsonResponse(baseStats)
  if (url.includes('/dating-users')) return jsonResponse({ users: [], total: 0 })
  if (url.includes('/users')) return jsonResponse({ users: [], total: 0 })
  if (url.includes('/bots/bot1')) return jsonResponse(baseBot)
  return null
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('tab in URL', () => {
  it('opens the tab named by ?tab= on first render', async () => {
    // Match the tickets endpoint BEFORE stubBase (its /bots/bot1 catch-all
    // would shadow it and crash TicketsTab on data.tickets).
    stubFetch((url) => {
      if (url.includes('/tickets')) return jsonResponse({ tickets: [], total: 0 })
      return stubBase(url) ?? jsonResponse({})
    })
    renderDetail('/bots/bot1?tab=tickets')

    // Tickets tab is active without any clicking — the URL decides the tab.
    const active = await screen.findByRole('button', { name: '📩 تیکت‌ها' })
    expect(active.className).toContain('text-brand-600')
    expect(screen.getByRole('button', { name: '👥 کاربران' }).className).not.toContain('text-brand-600')
    expect(await screen.findByText('تیکتی در این وضعیت نیست')).toBeInTheDocument()
  })

  it('falls back to the users tab for an unknown tab value', async () => {
    stubFetch((url) => {
      if (url.includes('/dating-users')) return jsonResponse({ users: [], total: 0 })
      if (url.includes('/users')) return jsonResponse({ users: [], total: 0 })
      return stubBase(url) ?? jsonResponse({})
    })
    renderDetail('/bots/bot1?tab=does-not-exist')

    const users = await screen.findByRole('button', { name: '👥 کاربران' })
    expect(users.className).toContain('text-brand-600')
    // default tab really mounted its content (empty users list, not a crash)
    expect(document.body.textContent).not.toContain('Failed to fetch')
  })

  it('writes the chosen tab into the URL', async () => {
    stubFetch((url) => {
      if (url.includes('/discounts')) return jsonResponse({ discounts: [] })
      return stubBase(url) ?? jsonResponse({})
    })
    const store = configureStore({
      reducer: { [sadbotApi.reducerPath]: sadbotApi.reducer },
      middleware: (getDefault) => getDefault().concat(sadbotApi.middleware),
    })
    function LocationProbe() {
      const { search } = useLocation()
      return <div data-testid="loc">{search}</div>
    }
    render(
      <Provider store={store}>
        <MemoryRouter initialEntries={['/bots/bot1']}>
          <Routes>
            <Route
              path="/bots/:id"
              element={
                <>
                  <BotDetailPage />
                  <LocationProbe />
                </>
              }
            />
          </Routes>
        </MemoryRouter>
      </Provider>,
    )
    const user = userEvent.setup()

    await user.click(await screen.findByRole('button', { name: /کدهای تخفیف/ }))
    expect(screen.getByTestId('loc')).toHaveTextContent('tab=discounts')
  })
})

describe('ConfessionsTab', () => {
  it('lists pending confessions and approves one', async () => {
    const calls: string[] = []
    stubFetch((url, init) => {
      const base = stubBase(url)
      if (base && !url.includes('confessions')) return base
      if (url.includes('/confessions/1/approve') && init?.method === 'POST') {
        calls.push(url)
        return jsonResponse({})
      }
      if (url.includes('/confessions')) {
        return jsonResponse({
          confessions: [
            { id: 1, bot_id: 'bot1', user_id: 7, text: 'اعتراف آزمایشی', status: 'pending', created_at: '2026-10-15T10:00:00Z' },
          ],
          total: 1,
        })
      }
      return jsonResponse({})
    })
    renderDetail()
    const user = userEvent.setup()

    await user.click(await screen.findByRole('button', { name: /اعتراف‌ها/ }))
    expect(await screen.findByText('اعتراف آزمایشی')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'تایید و انتشار' }))
    await vi.waitFor(() => expect(calls).toHaveLength(1))
    expect(calls[0]).toContain('/confessions/1/approve')
  })

  it('rejects a pending confession', async () => {
    const calls: string[] = []
    stubFetch((url, init) => {
      const base = stubBase(url)
      if (base && !url.includes('confessions')) return base
      if (url.includes('/confessions/2/reject') && init?.method === 'POST') {
        calls.push(url)
        return jsonResponse({})
      }
      if (url.includes('/confessions')) {
        return jsonResponse({
          confessions: [
            { id: 2, bot_id: 'bot1', user_id: 7, text: 'اعتراف برای رد', status: 'pending', created_at: '2026-10-15T10:00:00Z' },
          ],
          total: 1,
        })
      }
      return jsonResponse({})
    })
    renderDetail()
    const user = userEvent.setup()

    await user.click(await screen.findByRole('button', { name: /اعتراف‌ها/ }))
    await user.click(await screen.findByRole('button', { name: 'رد' }))
    await vi.waitFor(() => expect(calls).toHaveLength(1))
    expect(calls[0]).toContain('/confessions/2/reject')
  })

  it('shows the empty state when there are no confessions', async () => {
    stubFetch((url) => {
      const base = stubBase(url)
      if (base && !url.includes('confessions')) return base
      if (url.includes('/confessions')) return jsonResponse({ confessions: [], total: 0 })
      return jsonResponse({})
    })
    renderDetail()
    const user = userEvent.setup()

    await user.click(await screen.findByRole('button', { name: /اعتراف‌ها/ }))
    expect(await screen.findByText('اعترافی در این وضعیت نیست')).toBeInTheDocument()
  })
})

describe('TicketsTab', () => {
  it('sends an admin reply and closes a ticket', async () => {
    const calls: { url: string; body: unknown }[] = []
    stubFetch((url, init) => {
      const base = stubBase(url)
      if (base && !url.includes('tickets')) return base
      if (url.includes('/tickets/5/reply') && init?.method === 'POST') {
        calls.push({ url, body: init.body })
        return jsonResponse({})
      }
      if (url.includes('/tickets/5/close') && init?.method === 'POST') {
        calls.push({ url, body: null })
        return jsonResponse({})
      }
      if (url.includes('/tickets')) {
        return jsonResponse({
          tickets: [
            { id: 5, bot_id: 'bot1', user_id: 9, message: 'مشکل خرید سکه', status: 'open', created_at: '2026-10-15T09:00:00Z' },
          ],
          total: 1,
        })
      }
      return jsonResponse({})
    })
    renderDetail()
    const user = userEvent.setup()

    await user.click(await screen.findByRole('button', { name: /تیکت‌ها/ }))
    expect(await screen.findByText('مشکل خرید سکه')).toBeInTheDocument()

    await user.type(screen.getByPlaceholderText('پاسخ به کاربر…'), 'در حال بررسی هستیم.')
    await user.click(screen.getByRole('button', { name: 'ارسال پاسخ' }))
    await vi.waitFor(() => expect(calls).toHaveLength(1))
    expect(calls[0].url).toContain('/tickets/5/reply')
    expect(String(calls[0].body)).toContain('در حال بررسی هستیم.')

    await user.click(await screen.findByRole('button', { name: 'بستن' }))
    await vi.waitFor(() => expect(calls).toHaveLength(2))
    expect(calls[1].url).toContain('/tickets/5/close')
  })

  it('shows the empty state when there are no open tickets', async () => {
    stubFetch((url) => {
      const base = stubBase(url)
      if (base && !url.includes('tickets')) return base
      if (url.includes('/tickets')) return jsonResponse({ tickets: [], total: 0 })
      return jsonResponse({})
    })
    renderDetail()
    const user = userEvent.setup()

    await user.click(await screen.findByRole('button', { name: /تیکت‌ها/ }))
    expect(await screen.findByText('تیکتی در این وضعیت نیست')).toBeInTheDocument()
  })
})

describe('BroadcastTab', () => {
  it('shows audience count and posts a broadcast', async () => {
    const calls: { url: string; body: unknown }[] = []
    stubFetch((url, init) => {
      const base = stubBase(url)
      if (base && !url.includes('broadcast')) return base
      if (url.includes('/broadcast/preview')) return jsonResponse({ audience: 4 })
      if (url.includes('/broadcast/runs')) return jsonResponse({ runs: [] })
      if (url.includes('/broadcast') && init?.method === 'POST') {
        calls.push({ url, body: init.body })
        return jsonResponse(
          { id: 'r1', text: 'hi', audience: 4, sent: 0, failed: 0, status: 'running', created_at: '2026-10-16T10:00:00Z' },
          202,
        )
      }
      return jsonResponse({})
    })
    // confirm() would block jsdom — stub it
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    renderDetail()
    const user = userEvent.setup()

    await user.click(await screen.findByRole('button', { name: /اعلان/ }))
    expect(await screen.findByText(/کاربران مسدودشده حذف می‌شوند/)).toBeInTheDocument()

    await user.type(screen.getByPlaceholderText('متن اعلان…'), 'سلام از پنل')
    await user.click(screen.getByRole('button', { name: 'ارسال اعلان' }))

    await vi.waitFor(() => expect(calls).toHaveLength(1))
    expect(String(calls[0].body)).toContain('سلام از پنل')
  })
})

describe('DiscountsTab', () => {
  it('creates a discount code', async () => {
    const calls: { url: string; body: unknown }[] = []
    stubFetch((url, init) => {
      const base = stubBase(url)
      if (base && !url.includes('discounts')) return base
      if (url.includes('/discounts') && init?.method === 'POST') {
        calls.push({ url, body: init.body })
        return jsonResponse(
          { id: 'd1', code: 'OFF20', kind: 'percent', value: 20, max_uses: 0, used_count: 0, created_at: '2026-10-16T10:00:00Z' },
          201,
        )
      }
      if (url.includes('/discounts')) return jsonResponse({ discounts: [] })
      return jsonResponse({})
    })
    renderDetail()
    const user = userEvent.setup()

    await user.click(await screen.findByRole('button', { name: /کدهای تخفیف/ }))
    expect(await screen.findByText('کد تخفیفی ساخته نشده')).toBeInTheDocument()

    await user.type(screen.getByPlaceholderText('OFF50'), 'OFF20')
    await user.type(screen.getAllByRole('spinbutton')[0], '20')
    await user.click(screen.getByRole('button', { name: 'ساخت کد' }))

    await vi.waitFor(() => expect(calls).toHaveLength(1))
    expect(String(calls[0].body)).toContain('OFF20')
  })
})

describe('HealthTab', () => {
  it('lists stuck users and resets them', async () => {
    const calls: string[] = []
    stubFetch((url, init) => {
      if (url.includes('/admin/stuck/reset') && init?.method === 'POST') {
        calls.push(url)
        return jsonResponse({ reset: 3 })
      }
      if (url.includes('/admin/stuck')) {
        return jsonResponse({
          users: [
            {
              user_id: 5,
              telegram_id: 111,
              name: 'امیر',
              status: 'chatting',
              last_seen_at: '2026-10-15T10:00:00Z',
              stuck_minutes: 90,
            },
          ],
          total: 1,
        })
      }
      if (url.includes('/admin/blocks')) return jsonResponse({ blocks: [], total: 0 })
      return stubBase(url) ?? jsonResponse({})
    })
    renderDetail()
    const user = userEvent.setup()

    await user.click(await screen.findByRole('button', { name: /سلامت و ریست/ }))
    expect(await screen.findByText('امیر')).toBeInTheDocument()
    expect(screen.getByText('در چت')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'ریست گیرکرده‌ها' }))
    await vi.waitFor(() => expect(calls).toHaveLength(1))
    expect(calls[0]).toContain('/admin/stuck/reset')
  })

  it('shows the empty state when nobody is stuck', async () => {
    stubFetch((url) => {
      if (url.includes('/admin/blocks')) return jsonResponse({ blocks: [], total: 0 })
      if (url.includes('/admin/stuck')) return jsonResponse({ users: [], total: 0 })
      return stubBase(url) ?? jsonResponse({})
    })
    renderDetail()
    const user = userEvent.setup()

    await user.click(await screen.findByRole('button', { name: /سلامت و ریست/ }))
    expect(await screen.findByText(/کاربر گیرکرده‌ای نیست/)).toBeInTheDocument()
  })

  it('runs the telegram-block scan and lists block pairs', async () => {
    const scans: string[] = []
    stubFetch((url, init) => {
      if (url.includes('/telegram-blocks/scan') && init?.method === 'POST') {
        scans.push(String(init.body))
        return jsonResponse({ scanned: 2, blocked: 1, reachable: 1, failed: 0 })
      }
      if (url.includes('/admin/blocks')) {
        return jsonResponse({
          blocks: [
            { blocker_id: 111, blocker_name: 'امیر', blocked_id: 222, blocked_name: 'سارا', created_at: '2026-10-15T10:00:00Z' },
          ],
          total: 1,
        })
      }
      if (url.includes('/admin/stuck')) return jsonResponse({ users: [], total: 0 })
      return stubBase(url) ?? jsonResponse({})
    })
    renderDetail()
    const user = userEvent.setup()

    await user.click(await screen.findByRole('button', { name: /سلامت و ریست/ }))
    expect(await screen.findByText(/روابط بلاک کاربران/)).toBeInTheDocument()
    expect(await screen.findByText('امیر')).toBeInTheDocument()
    expect(screen.getByText('سارا')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'شروع اسکن' }))
    await vi.waitFor(() => expect(scans).toHaveLength(1))
    expect(JSON.parse(scans[0])).toEqual({ days: 7 })
    expect(await screen.findByText(/۱ بلاک/)).toBeInTheDocument()
  })
})

describe('SettingsTab', () => {
  it('renders numeric fields and saves changed values', async () => {
    const bodies: string[] = []
    stubFetch((url, init) => {
      if (url.includes('/settings') && init?.method === 'PUT') {
        bodies.push(String(init.body))
        return jsonResponse({ bot_id: 'bot1', version: 3 })
      }
      if (url.includes('/settings')) {
        return jsonResponse({
          fields: [
            { key: 'signup_bonus', label: 'هدیهٔ ثبت‌نام (سکه)', group: 'bonuses', kind: 'int', min: 0, max: 1000, value: 5 },
            { key: 'ghost_enabled', label: 'کاربران جعلی فعال', group: 'coldstart', kind: 'bool', min: 0, max: 1, value: 1 },
          ],
          groups: ['bonuses', 'coldstart'],
        })
      }
      if (url.includes('/config')) {
        return jsonResponse({ bot_id: 'bot1', texts: '{}', menus: '{}', prices: '{}', version: 2, updated_at: '' })
      }
      return stubBase(url) ?? jsonResponse({})
    })
    renderDetail()
    const user = userEvent.setup()

    await user.click(await screen.findByRole('button', { name: /تنظیمات عددی/ }))
    expect(await screen.findByText('هدیهٔ ثبت‌نام (سکه)')).toBeInTheDocument()

    const input = screen.getByDisplayValue('5')
    await user.clear(input)
    await user.type(input, '25')
    await user.click(screen.getByRole('button', { name: 'ذخیرهٔ تنظیمات' }))

    await vi.waitFor(() => expect(bodies).toHaveLength(1))
    const sent = JSON.parse(bodies[0]) as { version: number; values: Record<string, number> }
    expect(sent.values.signup_bonus).toBe(25)
    expect(sent.version).toBe(2)
  })
})

describe('UserDetailDrawer', () => {
  it('opens the drawer and applies a coin adjustment', async () => {
    const calls: { url: string; body: string }[] = []
    stubFetch((url, init) => {
      if (url.includes('/admin/users/5/coins') && init?.method === 'POST') {
        calls.push({ url, body: String(init.body) })
        return jsonResponse({ coins: 15 })
      }
      if (/\/admin\/users\/5$/.test(url.split('?')[0])) {
        return jsonResponse({
          user_id: 5,
          telegram_id: 111,
          first_name: 'امیر',
          is_blocked: false,
          first_seen_at: '2026-10-14T10:00:00Z',
          last_seen_at: '2026-10-15T10:00:00Z',
          has_profile: true,
          name: 'امیر',
          status: 'joined',
          coins: 5,
          streak_count: 2,
          likes_sent: 1,
          likes_received: 0,
          payments: [],
          recent_events: [],
        })
      }
      if (url.includes('/dating-users')) {
        return jsonResponse({
          users: [
            { user_id: 5, telegram_id: 111, first_name: 'امیر', last_seen_at: '2026-10-15T10:00:00Z', status: 'joined', coins: 5, is_ghost: false },
          ],
          total: 1,
        })
      }
      return stubBase(url) ?? jsonResponse({})
    })
    renderDetail()
    const user = userEvent.setup()

    await user.click(await screen.findByRole('button', { name: 'جزئیات' }))
    expect(await screen.findByText('وضعیت:', { exact: false })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '+۵۰' }))
    await vi.waitFor(() => expect(calls).toHaveLength(1))
    expect(calls[0].url).toContain('/admin/users/5/coins')
    expect(JSON.parse(calls[0].body).delta).toBe(50)
  })
})

describe('ConfigTab', () => {
  // Regression: the API serves config fields as jsonb objects once stored
  // correctly. The tab used to call JSON.parse() unconditionally and blew up
  // with "JSON Parse error: Unexpected identifier \"object\"".
  it('renders object-shaped config without crashing', async () => {
    stubFetch((url) => {
      if (url.includes('/config')) {
        return jsonResponse({
          bot_id: 'bot1',
          texts: { welcome: 'سلام' },
          menus: {},
          prices: { signup_bonus: 5, channels: ['@a'] },
          feature_flags: {},
          version: 4,
          updated_at: '2026-10-16T10:00:00Z',
        })
      }
      return stubBase(url) ?? jsonResponse({})
    })
    renderDetail()
    const user = userEvent.setup()

    await user.click(await screen.findByRole('button', { name: /تنظیمات و منو/ }))
    // the textarea must hold pretty-printed JSON, not crash the route
    const area = await screen.findByDisplayValue(/"signup_bonus": 5/)
    expect(area).toBeInTheDocument()
    expect(screen.queryByText(/Application Error/)).not.toBeInTheDocument()
  })

  it('still renders string-shaped config (legacy rows)', async () => {
    stubFetch((url) => {
      if (url.includes('/config')) {
        return jsonResponse({
          bot_id: 'bot1',
          texts: '{"welcome":"سلام"}',
          menus: '{}',
          prices: '{"signup_bonus":5}',
          feature_flags: '{}',
          version: 1,
          updated_at: '',
        })
      }
      return stubBase(url) ?? jsonResponse({})
    })
    renderDetail()
    const user = userEvent.setup()

    await user.click(await screen.findByRole('button', { name: /تنظیمات و منو/ }))
    expect(await screen.findByDisplayValue(/"signup_bonus": 5/)).toBeInTheDocument()
  })
})

describe('ChatsTab', () => {
  it('lists active conversations and opens a transcript', async () => {
    const calls: string[] = []
    stubFetch((url) => {
      if (url.includes('/admin/chats/transcript')) {
        calls.push(url)
        return jsonResponse({
          messages: [
            { event_id: 1, user_id: 10, telegram_id: 111, name: 'امیر', is_user_a: true, kind: 'inbound_message', text: 'سلام', created_at: '2026-10-16T10:00:00Z' },
            { event_id: 2, user_id: 20, telegram_id: 222, name: 'سارا', is_user_a: false, kind: 'inbound_message', text: 'سلام! چطوری؟', created_at: '2026-10-16T10:01:00Z' },
          ],
          total: 2,
        })
      }
      if (url.includes('/admin/chats')) {
        return jsonResponse({
          chats: [
            {
              user_a_id: 10, telegram_a_id: 111, name_a: 'امیر', username_a: null,
              user_b_id: 20, telegram_b_id: 222, name_b: 'سارا', username_b: null,
              last_seen_at: '2026-10-16T10:00:00Z', status: 'chatting',
              is_ghost_a: false, is_ghost_b: false,
            },
          ],
          total: 1,
        })
      }
      if (url.includes('/admin/messages')) {
        return jsonResponse({
          messages: [{ event_id: 1, user_id: 10, telegram_id: 111, name: 'امیر', text: 'سلام', kind: 'inbound_message', created_at: '2026-10-16T10:00:00Z' }],
          total: 1,
        })
      }
      return stubBase(url) ?? jsonResponse({})
    })
    renderDetail()
    const user = userEvent.setup()

    await user.click(await screen.findByRole('button', { name: /چت‌ها/ }))
    // both names appear in the active-chats table
    expect(await screen.findByText('سارا')).toBeInTheDocument()
    expect(screen.getAllByText('امیر').length).toBeGreaterThan(0)

    await user.click(screen.getByRole('button', { name: 'مشاهدهٔ مکالمه' }))
    expect(await screen.findByText('سلام! چطوری؟')).toBeInTheDocument()
    await vi.waitFor(() => expect(calls).toHaveLength(1))
    expect(calls[0]).toContain('/admin/chats/transcript?a=10&b=20')
  })

  it('shows the empty state when there are no active chats', async () => {
    stubFetch((url) => {
      if (url.includes('/admin/chats')) return jsonResponse({ chats: [], total: 0 })
      if (url.includes('/admin/messages')) return jsonResponse({ messages: [], total: 0 })
      return stubBase(url) ?? jsonResponse({})
    })
    renderDetail()
    const user = userEvent.setup()

    await user.click(await screen.findByRole('button', { name: /چت‌ها/ }))
    expect(await screen.findByText('چت فعالی در جریان نیست')).toBeInTheDocument()
  })
})

describe('ChannelsTab', () => {
  it('lists channels, adds one, and saves via PUT', async () => {
    const bodies: string[] = []
    stubFetch((url, init) => {
      if (url.includes('/admin/channels') && init?.method === 'PUT') {
        bodies.push(String(init.body))
        return jsonResponse({ bot_id: 'bot1', version: 4 })
      }
      if (url.includes('/admin/channels')) {
        return jsonResponse({ channels: [{ id: '@main_channel', title: 'کانال اصلی', link: 'https://t.me/main_channel' }] })
      }
      if (url.includes('/config')) {
        return jsonResponse({ bot_id: 'bot1', texts: '{}', menus: '{}', prices: '{}', version: 3, updated_at: '' })
      }
      return stubBase(url) ?? jsonResponse({})
    })
    renderDetail()
    const user = userEvent.setup()

    await user.click(await screen.findByRole('button', { name: /کانال‌ها/ }))
    expect(await screen.findByDisplayValue('@main_channel')).toBeInTheDocument()
    expect(screen.getByDisplayValue('کانال اصلی')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /افزودن کانال/ }))
    const inputs = screen.getAllByPlaceholderText('@channel or -100123…')
    await user.type(inputs[1], '@second_channel')
    await user.click(screen.getByRole('button', { name: 'ذخیره' }))

    await vi.waitFor(() => expect(bodies).toHaveLength(1))
    const sent = JSON.parse(bodies[0]) as { version: number; channels: Array<{ id: string; title?: string }> }
    expect(sent.version).toBe(3)
    expect(sent.channels.map((c) => c.id)).toEqual(['@main_channel', '@second_channel'])
    expect(sent.channels[0].title).toBe('کانال اصلی')
  })

  it('shows the empty state when no channels exist', async () => {
    stubFetch((url) => {
      if (url.includes('/admin/channels')) return jsonResponse({ channels: [] })
      return stubBase(url) ?? jsonResponse({})
    })
    renderDetail()
    await userEvent.setup().click(await screen.findByRole('button', { name: /کانال‌ها/ }))
    expect(await screen.findByText(/کانالی ثبت نشده/)).toBeInTheDocument()
  })
})

describe('AITab', () => {
  it('lists AI conversations, opens history, and toggles the switch', async () => {
    const puts: string[] = []
    stubFetch((url, init) => {
      if (url.includes('/admin/ai/users/7')) {
        return jsonResponse({
          user_id: 7,
          telegram_id: 700,
          history: [
            { role: 'user', content: 'سلام' },
            { role: 'assistant', content: 'سلام! حالت چطوره؟' },
          ],
        })
      }
      if (url.includes('/admin/ai') && init?.method === 'PUT') {
        puts.push(String(init.body))
        return jsonResponse({ bot_id: 'bot1', version: 5 })
      }
      if (url.includes('/admin/ai')) {
        return jsonResponse({
          enabled: true,
          conversations: [
            {
              user_id: 7,
              telegram_id: 700,
              first_name: 'علی',
              last_seen_at: '2026-10-16T10:00:00Z',
              is_blocked: false,
              is_ghost: false,
              ai_free_used: 3,
              messages: 2,
              last_message: 'سلام!',
            },
          ],
        })
      }
      return stubBase(url) ?? jsonResponse({})
    })
    renderDetail()
    const user = userEvent.setup()

    await user.click(await screen.findByRole('button', { name: /دستیار هوشمند/ }))
    expect(await screen.findByText('علی')).toBeInTheDocument()
    expect(screen.getByText(/● فعال/)).toBeInTheDocument()

    // open the per-user history
    await user.click(screen.getByRole('button', { name: 'تاریخچه' }))
    expect(await screen.findByText(/سلام! حالت چطوره؟/)).toBeInTheDocument()

    // toggle off
    await user.click(screen.getByRole('button', { name: 'غیرفعال کردن' }))
    await vi.waitFor(() => expect(puts).toHaveLength(1))
    expect(JSON.parse(puts[0])).toEqual({ enabled: false })
  })

  it('shows the empty state when nobody is in the AI scene', async () => {
    stubFetch((url) => {
      if (url.includes('/admin/ai')) return jsonResponse({ enabled: false, conversations: [] })
      return stubBase(url) ?? jsonResponse({})
    })
    renderDetail()
    await userEvent.setup().click(await screen.findByRole('button', { name: /دستیار هوشمند/ }))
    expect(await screen.findByText(/کاربری الان در صحنهٔ AI نیست/)).toBeInTheDocument()
    expect(screen.getByText(/○ غیرفعال/)).toBeInTheDocument()
  })
})

describe('content bot', () => {
  const contentBot = {
    bot: { ...baseBot.bot, template_key: 'content', name: 'آقای تریدر' },
    running: true,
  }

  const contentSettings = {
    version: 3,
    account_id: 'acc1',
    folder: 'Crypto',
    channel: '-1004410420822',
    publish_via: 'bot',
    exclude: ['music'],
    keywords: [],
    poll_minutes: 2,
    backfill_limit: 20,
    min_length: 40,
    skip_forwarded: false,
    skip_replies: false,
    render_card_summary: true,
    text_only: false,
    brand_key: 'mr',
    brand_name_en: 'mr.trade.ai',
    brand_name_fa: 'آقای تریدر',
    brand_handle: '@mr.trade.ai',
    brand_glow_a: 'rgba(239, 68, 68, 0.28)',
    brand_glow_b: 'rgba(249, 115, 22, 0.20)',
    brand_glow_c: 'rgba(251, 146, 60, 0.12)',
    brand_accent: '#FFA070',
    clean_prompt: 'پاک‌سازی کن',
    card_prompt: 'خلاصه کن',
    default_clean_prompt: 'DEFAULT_CLEAN',
    default_card_prompt: 'DEFAULT_CARD',
    accounts: [
      { id: 'panel-acc', label: 'DA', sidecar_account_id: 'acc1', configured: true },
    ],
  }

  function stubContent(url: string): Response | null {
    if (url.includes('/content/settings')) return jsonResponse(contentSettings)
    if (url.includes('/content/posts')) {
      return jsonResponse({
        posts: [
          {
            id: 1, bot_id: 'bot1', source_chat_id: -1001, message_id: 55,
            source_title: 'کریپتو نیوز', source_link: 'https://t.me/abc/55',
            cleaned_text: 'متن تمیزشده', status: 'published', created_at: '2026-10-21T10:00:00Z',
          },
        ],
        total: 1,
        counts: { published: 1 },
        last_24h: 1,
      })
    }
    if (url.includes('/content/folders')) {
      return jsonResponse({ folders: [{ name: 'Crypto', peers: [] }] })
    }
    if (url.includes('/content/chats')) {
      return jsonResponse({ chats: [{ chat_id: '-1009', title: 'Crypto-Content', type: 'channel' }] })
    }
    if (url.includes('/telegram-accounts')) return jsonResponse({ accounts: [] })
    if (url.includes('/stats')) return jsonResponse(baseStats)
    if (url.includes('/bots/bot1')) return jsonResponse(contentBot)
    return null
  }

  async function renderContent(path = '/bots/bot1?tab=content') {
    stubFetch((url) => stubContent(url) ?? jsonResponse({}))
    renderDetail(path)
  }

  it('shows the content tab by default and hides dating tabs', async () => {
    await renderContent('/bots/bot1?tab=users') // users is not applicable → falls back
    expect(await screen.findByTestId('content-tab')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '👥 کاربران' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '💌 اعتراف‌ها' })).not.toBeInTheDocument()
    const active = await screen.findByRole('button', { name: /محتوا و رخدادها/ })
    expect(active.className).toContain('text-brand-600')
  })

  it('renders stats, source form and ledger rows', async () => {
    await renderContent()
    expect(await screen.findByTestId('content-stats')).toBeInTheDocument()
    expect(screen.getByTestId('content-form')).toBeInTheDocument()
    // source picker carries the current account/channel
    expect(screen.getByDisplayValue('Crypto')).toBeInTheDocument()
    // ledger goes loading→data: wait for the collected post itself
    expect(await screen.findByText('کریپتو نیوز')).toBeInTheDocument()
    const ledger = screen.getByTestId('content-ledger')
    expect(ledger).toHaveTextContent('منتشرشده')
    expect(ledger).toHaveTextContent('متن تمیزشده')
  })

  it('hides the dating stat boxes on content bots', async () => {
    await renderContent()
    expect(await screen.findByTestId('content-tab')).toBeInTheDocument()
    expect(screen.queryByText('کاربر جدید (۳۰ روز)')).not.toBeInTheDocument()
  })

  it('opens the clone modal', async () => {
    await renderContent()
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: 'کلون بات' }))
    const dialog = await screen.findByRole('dialog')
    expect(dialog).toHaveTextContent('کلون بات محتوا')
    expect(screen.getByDisplayValue('آقای تریدر (کلون)')).toBeInTheDocument()
  })

  it('keeps dating bots on the classic tab set', async () => {
    stubFetch((url) => {
      if (url.includes('/dating-users')) return jsonResponse({ users: [], total: 0 })
      if (url.includes('/users')) return jsonResponse({ users: [], total: 0 })
      return stubBase(url) ?? jsonResponse({})
    })
    renderDetail('/bots/bot1')
    expect(await screen.findByRole('button', { name: '👥 کاربران' })).toBeInTheDocument()
    expect(screen.queryByTestId('content-tab')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'کلون بات' })).not.toBeInTheDocument()
  })
})
