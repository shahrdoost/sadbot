import { createApi, fetchBaseQuery } from '@reduxjs/toolkit/query/react'

const baseQuery = fetchBaseQuery({
  baseUrl: '/api/v1',
  prepareHeaders: (headers) => {
    const token = localStorage.getItem('sadbot_token')
    if (token) headers.set('Authorization', `Bearer ${token}`)
    return headers
  },
})

export interface Admin {
  id: string
  email: string
  display_name: string
  role: 'superadmin' | 'admin'
  status: 'active' | 'disabled'
  created_at: string
}

export interface Bot {
  id: string
  name: string
  telegram_username?: string
  template_key: string
  status: string
  last_error?: string
  created_at: string
  running?: boolean
}

export interface BotUser {
  id: number
  telegram_id: number
  username?: string
  first_name?: string
  last_name?: string
  state: string
  is_blocked: boolean
  first_seen_at: string
  last_seen_at: string
}

export interface BotEvent {
  id: number
  user_id: number
  username?: string
  first_name?: string
  kind: string
  text?: string
  created_at: string
}

export interface Payment {
  id: string
  user_id?: number
  username?: string
  first_name?: string
  amount_rials: number
  kind: string
  gateway: string
  status: string
  note?: string
  created_at: string
}

export interface FinanceOverview {
  total_revenue_rials: number
  total_users: number
  active_subscriptions: number
  per_bot: Array<{
    bot_id: string
    bot_name: string
    revenue_rials: number
    users: number
    active_subscriptions: number
  }>
}

export interface BotConfig {
  bot_id: string
  // jsonb columns: served as objects once stored correctly, but older rows
  // may still hold a JSON-encoded string, so callers must handle both
  texts: unknown
  menus: unknown
  feature_flags: unknown
  prices: unknown
  version: number
  updated_at: string
}

export interface BotStats {
  total_users: number
  new_users_30d: number
  revenue_30d_rials: number
  active_subscriptions: number
  events_24h: number
}

// ---- content template panel ----

export interface ContentAccount {
  id: string
  label: string
  username?: string
  phone?: string
  sidecar_account_id: string
  configured: boolean
}

export interface ContentSettings {
  version: number
  account_id: string
  folder: string
  channel: string
  publish_via: 'bot' | 'sidecar'
  exclude: string[]
  keywords: string[]
  poll_minutes: number
  backfill_limit: number
  min_length: number
  skip_forwarded: boolean
  skip_replies: boolean
  render_card_summary: boolean
  text_only: boolean
  brand_key: string
  brand_name_en: string
  brand_name_fa: string
  brand_handle: string
  brand_glow_a: string
  brand_glow_b: string
  brand_glow_c: string
  brand_accent: string
  clean_prompt: string
  card_prompt: string
  default_clean_prompt: string
  default_card_prompt: string
  accounts: ContentAccount[]
}

// PUT body: version + every keyed field above minus the view-only extras
export type ContentSettingsPut = Omit<
  ContentSettings,
  'default_clean_prompt' | 'default_card_prompt' | 'accounts'
>

export interface ContentFolder {
  name: string
  peers: Array<{ chat_id: string; title: string; username: string; type: string }>
}

export interface ContentPost {
  id: number
  bot_id: string
  source_chat_id: number
  message_id: number
  source_title?: string
  source_link?: string
  raw_text?: string
  cleaned_text?: string
  status: 'published' | 'no_content' | 'failed' | 'render_failed'
  error?: string
  created_at: string
}

export interface ContentPostsPage {
  posts: ContentPost[]
  total: number
  counts: Record<string, number>
  last_24h: number
}

export interface ContentChat {
  chat_id: string
  title: string
  type: string
  unread?: number
  pinned?: boolean
}

export interface CloneResult {
  bot: Bot
  config_error?: string
}

export interface Confession {
  id: number
  bot_id: string
  user_id: number
  username?: string
  first_name?: string
  text: string
  status: 'pending' | 'approved' | 'rejected'
  created_at: string
}

export interface SupportTicket {
  id: number
  bot_id: string
  user_id: number
  username?: string
  first_name?: string
  message: string
  admin_reply?: string
  status: 'open' | 'answered' | 'closed'
  created_at: string
}

export interface DatingUser {
  user_id: number
  telegram_id: number
  username?: string
  first_name?: string
  last_seen_at: string
  status?: string
  coins?: number
  rating?: number
  total_ratings?: number
  is_ghost: boolean
  ghost_persona?: string
}

export interface BlockPair {
  blocker_id: number
  blocker_name: string
  blocked_id: number
  blocked_name: string
  created_at: string
}

export interface ScanResult {
  scanned: number
  blocked: number
  reachable: number
  failed: number
}

export interface BroadcastRun {
  id: string
  text: string
  photo?: string
  footer?: string
  audience: number
  sent: number
  failed: number
  status: 'running' | 'done'
  created_at: string
  finished_at?: string
}

export interface DiscountCode {
  id: string
  code: string
  kind: 'percent' | 'flat_toman' | 'coin_bonus'
  value: number
  max_uses: number
  used_count: number
  expires_at?: string
  created_at: string
}

export interface StuckUser {
  user_id: number
  telegram_id: number
  name?: string
  username?: string
  status: string
  partner_user_id?: number
  last_seen_at: string
  stuck_minutes: number
}

export interface UserDetail {
  user_id: number
  telegram_id: number
  username?: string
  first_name?: string
  last_name?: string
  is_blocked: boolean
  first_seen_at: string
  last_seen_at: string
  has_profile: boolean
  name?: string
  gender?: string
  age?: number
  city?: string
  status?: string
  partner_user_id?: number
  coins?: number
  rating?: number
  total_ratings?: number
  streak_count?: number
  streak_best?: number
  boosted_until?: string
  profile_blocked_at?: string
  blocked_reason?: string
  likes_sent: number
  likes_received: number
  payments: Array<{
    id: string
    amount_rials: number
    kind: string
    gateway: string
    status: string
    note?: string
    created_at: string
  }>
  recent_events: Array<{ id: number; kind: string; text?: string; created_at: string }>
}

export interface SettingField {
  key: string
  label: string
  group: string
  kind: 'int' | 'bool'
  min: number
  max: number
  value: number
}

export interface ActiveChat {
  user_a_id: number
  telegram_a_id: number
  name_a?: string
  username_a?: string
  user_b_id: number
  telegram_b_id: number
  name_b?: string
  username_b?: string
  last_seen_at: string
  status: string
  is_ghost_a: boolean
  is_ghost_b: boolean
}

export interface TranscriptEntry {
  event_id: number
  user_id: number
  telegram_id: number
  name?: string
  is_user_a: boolean
  kind: string
  text?: string
  meta?: unknown
  created_at: string
}

export interface ChatPreview {
  event_id: number
  user_id: number
  telegram_id: number
  name?: string
  username?: string
  partner_user_id?: number
  partner_name?: string
  partner_username?: string
  text: string
  kind: string
  is_outbound?: boolean
  created_at: string
}

export interface ChannelEntry {
  id: string
  title?: string
  link?: string
}

export interface AIConversation {
  user_id: number
  telegram_id: number
  username?: string
  first_name?: string
  last_seen_at: string
  is_blocked: boolean
  is_ghost: boolean
  ai_free_used: number
  messages: number
  last_message?: string
  last_at?: string
}

export interface AIHarness {
  enabled: boolean
  conversations: AIConversation[]
}

export interface AIDetail {
  user_id: number
  telegram_id: number
  username?: string
  first_name?: string
  last_seen_at: string
  ai_free_used?: number
  is_ghost?: boolean
  history: Array<{ role: string; content: string; at?: string }>
}

export const sadbotApi = createApi({
  reducerPath: 'sadbotApi',
  baseQuery,
  tagTypes: ['Bot', 'BotUser', 'Event', 'Payment', 'Config', 'Admin', 'Finance', 'Stats', 'TgAccount', 'TgChat', 'Confession', 'Ticket', 'DatingUser', 'Broadcast', 'Discount', 'AdminOps', 'Chats', 'Channels', 'AI', 'Blocks', 'ContentSettings', 'ContentPosts'],
  endpoints: (builder) => ({
    login: builder.mutation<{ access_token: string; admin: Admin }, { email: string; password: string }>({
      query: (body) => ({ url: '/auth/login', method: 'POST', body }),
    }),
    me: builder.query<Admin, void>({
      query: () => '/auth/me',
    }),
    listBots: builder.query<{ bots: Bot[] }, void>({
      query: () => '/bots',
      providesTags: ['Bot'],
    }),
    createBot: builder.mutation<Bot, { name: string; token: string; template_key: string }>({
      query: (body) => ({ url: '/bots', method: 'POST', body }),
      invalidatesTags: ['Bot', 'Finance'],
    }),
    createBotFromAccount: builder.mutation<
      Bot,
      { account_id: string; name: string; template_key: string; username_hint?: string }
    >({
      query: ({ account_id, ...body }) => ({
        url: `/telegram-accounts/${account_id}/create-bot`,
        method: 'POST',
        body,
      }),
      invalidatesTags: ['Bot', 'Finance'],
    }),
    getBot: builder.query<{ bot: Bot; running: boolean }, string>({
      query: (id) => `/bots/${id}`,
      providesTags: (_r, _e, id) => [{ type: 'Bot', id }],
    }),
    deleteBot: builder.mutation<unknown, string>({
      query: (id) => ({ url: `/bots/${id}`, method: 'DELETE' }),
      invalidatesTags: ['Bot', 'Finance'],
    }),
    startBot: builder.mutation<unknown, string>({
      query: (id) => ({ url: `/bots/${id}/start`, method: 'POST' }),
      invalidatesTags: (_r, _e, id) => [{ type: 'Bot', id }],
    }),
    stopBot: builder.mutation<unknown, string>({
      query: (id) => ({ url: `/bots/${id}/stop`, method: 'POST' }),
      invalidatesTags: (_r, _e, id) => [{ type: 'Bot', id }],
    }),
    getBotStats: builder.query<BotStats, string>({
      query: (id) => `/bots/${id}/stats`,
      providesTags: (_r, _e, id) => [{ type: 'Stats', id }],
    }),
    listBotUsers: builder.query<{ users: BotUser[]; total: number }, { botId: string; q?: string; page?: number }>({
      query: ({ botId, ...params }) => ({
        url: `/bots/${botId}/users`,
        params,
      }),
      providesTags: (_r, _e, { botId }) => [{ type: 'BotUser', botId }],
    }),
    listEvents: builder.query<{ events: BotEvent[]; total: number }, { botId: string; kind?: string; page?: number }>({
      query: ({ botId, ...params }) => ({
        url: `/bots/${botId}/events`,
        params,
      }),
      providesTags: (_r, _e, { botId }) => [{ type: 'Event', botId }],
    }),
    listPayments: builder.query<{ payments: Payment[]; total: number }, { botId: string; page?: number }>({
      query: ({ botId, page }) => ({
        url: `/bots/${botId}/payments`,
        params: { page },
      }),
      providesTags: (_r, _e, { botId }) => [{ type: 'Payment', botId }],
    }),
    createPayment: builder.mutation<Payment, { botId: string; user_id?: number; amount_rials: number; note?: string }>({
      query: ({ botId, ...body }) => ({
        url: `/bots/${botId}/payments`,
        method: 'POST',
        body,
      }),
      invalidatesTags: (_r, _e, { botId }) => [{ type: 'Payment', botId }, 'Finance', { type: 'Stats', id: botId }],
    }),
    getConfig: builder.query<BotConfig, string>({
      query: (id) => `/bots/${id}/config`,
      providesTags: (_r, _e, id) => [{ type: 'Config', id }],
    }),
    putConfig: builder.mutation<BotConfig, { id: string; version: number; texts: string; menus: string; prices: string }>({
      query: ({ id, ...body }) => ({
        url: `/bots/${id}/config`,
        method: 'PUT',
        body,
      }),
      invalidatesTags: (_r, _e, { id }) => [{ type: 'Config', id }],
    }),
    // — content template panel —
    getContentSettings: builder.query<ContentSettings, string>({
      query: (botId) => `/bots/${botId}/content/settings`,
      providesTags: (_r, _e, botId) => [{ type: 'ContentSettings', id: botId }],
    }),
    putContentSettings: builder.mutation<BotConfig, { botId: string; body: ContentSettingsPut }>({
      query: ({ botId, body }) => ({ url: `/bots/${botId}/content/settings`, method: 'PUT', body }),
      invalidatesTags: (_r, _e, { botId }) => [
        { type: 'ContentSettings', id: botId },
        { type: 'ContentPosts', id: botId },
        { type: 'Config', id: botId },
      ],
    }),
    getContentFolders: builder.query<{ folders: ContentFolder[] }, { botId: string; account: string }>({
      query: ({ botId, account }) => `/bots/${botId}/content/folders?account=${encodeURIComponent(account)}`,
      providesTags: (_r, _e, { botId }) => [{ type: 'ContentSettings', id: botId }],
    }),
    getContentChats: builder.query<{ chats: ContentChat[] }, { botId: string; account: string }>({
      query: ({ botId, account }) => `/bots/${botId}/content/chats?account=${encodeURIComponent(account)}`,
      providesTags: (_r, _e, { botId }) => [{ type: 'ContentSettings', id: botId }],
    }),
    getContentPosts: builder.query<ContentPostsPage, { botId: string; page?: number; size?: number; status?: string; q?: string }>({
      query: ({ botId, ...params }) => ({ url: `/bots/${botId}/content/posts`, params }),
      providesTags: (_r, _e, { botId }) => [{ type: 'ContentPosts', id: botId }],
    }),
    cloneBot: builder.mutation<CloneResult, { botId: string; account_id: string; name?: string; username_hint?: string }>({
      query: ({ botId, ...body }) => ({ url: `/bots/${botId}/clone`, method: 'POST', body }),
      invalidatesTags: ['Bot', 'Finance'],
    }),
    financeOverview: builder.query<FinanceOverview, void>({
      query: () => '/finance/overview',
      providesTags: ['Finance'],
    }),
    listAdmins: builder.query<{ admins: Admin[] }, void>({
      query: () => '/admins',
      providesTags: ['Admin'],
    }),
    createAdmin: builder.mutation<Admin, { email: string; password: string; display_name: string; role: string }>({
      query: (body) => ({ url: '/admins', method: 'POST', body }),
      invalidatesTags: ['Admin'],
    }),
    deleteAdmin: builder.mutation<unknown, string>({
      query: (id) => ({ url: `/admins/${id}`, method: 'DELETE' }),
      invalidatesTags: ['Admin'],
    }),
    setBotAccess: builder.mutation<unknown, { adminId: string; botId: string; permissions: string[] }>({
      query: ({ adminId, botId, ...body }) => ({
        url: `/admins/${adminId}/bot-access/${botId}`,
        method: 'PUT',
        body,
      }),
    }),
    listConfessions: builder.query<{ confessions: Confession[]; total: number }, { botId: string; status?: string; page?: number }>({
      query: ({ botId, ...params }) => ({ url: `/bots/${botId}/confessions`, params }),
      providesTags: (_r, _e, { botId }) => [{ type: 'Confession', botId }],
    }),
    approveConfession: builder.mutation<unknown, { botId: string; id: number }>({
      query: ({ botId, id }) => ({ url: `/bots/${botId}/confessions/${id}/approve`, method: 'POST' }),
      invalidatesTags: (_r, _e, { botId }) => [{ type: 'Confession', botId }],
    }),
    rejectConfession: builder.mutation<unknown, { botId: string; id: number }>({
      query: ({ botId, id }) => ({ url: `/bots/${botId}/confessions/${id}/reject`, method: 'POST' }),
      invalidatesTags: (_r, _e, { botId }) => [{ type: 'Confession', botId }],
    }),
    seedConfessions: builder.mutation<{ inserted: number }, string>({
      query: (botId) => ({ url: `/bots/${botId}/confessions/seed`, method: 'POST' }),
      invalidatesTags: (_r, _e, botId) => [{ type: 'Confession', botId }],
    }),
    listTickets: builder.query<{ tickets: SupportTicket[]; total: number }, { botId: string; status?: string; page?: number }>({
      query: ({ botId, ...params }) => ({ url: `/bots/${botId}/tickets`, params }),
      providesTags: (_r, _e, { botId }) => [{ type: 'Ticket', botId }],
    }),
    replyTicket: builder.mutation<unknown, { botId: string; id: number; reply: string }>({
      query: ({ botId, id, ...body }) => ({ url: `/bots/${botId}/tickets/${id}/reply`, method: 'POST', body }),
      invalidatesTags: (_r, _e, { botId }) => [{ type: 'Ticket', botId }],
    }),
    closeTicket: builder.mutation<unknown, { botId: string; id: number }>({
      query: ({ botId, id }) => ({ url: `/bots/${botId}/tickets/${id}/close`, method: 'POST' }),
      invalidatesTags: (_r, _e, { botId }) => [{ type: 'Ticket', botId }],
    }),
    listDatingUsers: builder.query<{ users: DatingUser[]; total: number }, { botId: string; q?: string; page?: number; ghost?: string }>({
      query: ({ botId, ...params }) => ({ url: `/bots/${botId}/dating-users`, params }),
      providesTags: (_r, _e, { botId }) => [{ type: 'DatingUser', botId }],
    }),
    broadcastPreview: builder.query<{ audience: number }, string>({
      query: (botId) => `/bots/${botId}/broadcast/preview`,
    }),
    startBroadcast: builder.mutation<BroadcastRun, { botId: string; text: string; photo?: string; footer?: string }>({
      query: ({ botId, ...body }) => ({ url: `/bots/${botId}/broadcast`, method: 'POST', body }),
      invalidatesTags: (_r, _e, { botId }) => [{ type: 'Broadcast', botId }],
    }),
    listBroadcasts: builder.query<{ runs: BroadcastRun[] }, string>({
      query: (botId) => `/bots/${botId}/broadcast/runs`,
      providesTags: (_r, _e, botId) => [{ type: 'Broadcast', botId }],
    }),
    listDiscounts: builder.query<{ discounts: DiscountCode[] }, string>({
      query: (botId) => `/bots/${botId}/discounts`,
      providesTags: (_r, _e, botId) => [{ type: 'Discount', botId }],
    }),
    createDiscount: builder.mutation<
      DiscountCode,
      { botId: string; code: string; kind: string; value: number; max_uses?: number; expires_at?: string }
    >({
      query: ({ botId, ...body }) => ({ url: `/bots/${botId}/discounts`, method: 'POST', body }),
      invalidatesTags: (_r, _e, { botId }) => [{ type: 'Discount', botId }],
    }),
    deleteDiscount: builder.mutation<unknown, { botId: string; id: string }>({
      query: ({ botId, id }) => ({ url: `/bots/${botId}/discounts/${id}`, method: 'DELETE' }),
      invalidatesTags: (_r, _e, { botId }) => [{ type: 'Discount', botId }],
    }),
    getUserDetail: builder.query<UserDetail, { botId: string; userId: number }>({
      query: ({ botId, userId }) => `/bots/${botId}/admin/users/${userId}`,
      providesTags: (_r, _e, { botId, userId }) => [{ type: 'AdminOps', id: `${botId}:${userId}` }],
    }),
    adjustCoins: builder.mutation<{ coins: number }, { botId: string; userId: number; delta: number }>({
      query: ({ botId, userId, ...body }) => ({
        url: `/bots/${botId}/admin/users/${userId}/coins`,
        method: 'POST',
        body,
      }),
      invalidatesTags: (_r, _e, { botId, userId }) => [
        { type: 'AdminOps', id: `${botId}:${userId}` },
        { type: 'BotUser', botId },
        { type: 'DatingUser', botId },
      ],
    }),
    resetUser: builder.mutation<unknown, { botId: string; userId: number }>({
      query: ({ botId, userId }) => ({ url: `/bots/${botId}/admin/users/${userId}/reset`, method: 'POST' }),
      invalidatesTags: (_r, _e, { botId, userId }) => [
        { type: 'AdminOps', id: `${botId}:${userId}` },
        { type: 'BotUser', botId },
        { type: 'DatingUser', botId },
        { type: 'Stats', id: botId },
      ],
    }),
    setUserBlocked: builder.mutation<unknown, { botId: string; userId: number; blocked: boolean; reason?: string }>({
      query: ({ botId, userId, ...body }) => ({
        url: `/bots/${botId}/admin/users/${userId}/block`,
        method: 'POST',
        body,
      }),
      invalidatesTags: (_r, _e, { botId, userId }) => [
        { type: 'AdminOps', id: `${botId}:${userId}` },
        { type: 'BotUser', botId },
        { type: 'DatingUser', botId },
      ],
    }),
    resetStreak: builder.mutation<unknown, { botId: string; userId: number }>({
      query: ({ botId, userId }) => ({ url: `/bots/${botId}/admin/users/${userId}/streak`, method: 'POST' }),
      invalidatesTags: (_r, _e, { botId, userId }) => [{ type: 'AdminOps', id: `${botId}:${userId}` }],
    }),
    directMessage: builder.mutation<unknown, { botId: string; userId: number; text: string }>({
      query: ({ botId, userId, text }) => ({
        url: `/bots/${botId}/admin/users/${userId}/message`,
        method: 'POST',
        body: { text },
      }),
      invalidatesTags: (_r, _e, { botId, userId }) => [{ type: 'AdminOps', id: `${botId}:${userId}` }],
    }),
    listStuckUsers: builder.query<{ users: StuckUser[]; total: number }, string>({
      query: (botId) => `/bots/${botId}/admin/stuck`,
      providesTags: (_r, _e, botId) => [{ type: 'AdminOps', id: `${botId}:stuck` }],
    }),
    resetStuck: builder.mutation<{ reset: number }, string>({
      query: (botId) => ({ url: `/bots/${botId}/admin/stuck/reset`, method: 'POST' }),
      invalidatesTags: (_r, _e, botId) => [
        { type: 'AdminOps', id: `${botId}:stuck` },
        { type: 'DatingUser', botId },
        { type: 'BotUser', botId },
      ],
    }),
    resetAllChats: builder.mutation<{ reset: number }, string>({
      query: (botId) => ({
        url: `/bots/${botId}/admin/reset-all`,
        method: 'POST',
        body: { confirm: 'confirm' },
      }),
      invalidatesTags: (_r, _e, botId) => [
        { type: 'AdminOps', id: `${botId}:stuck` },
        { type: 'DatingUser', botId },
        { type: 'BotUser', botId },
        { type: 'Stats', id: botId },
      ],
    }),
    getSettings: builder.query<{ fields: SettingField[]; groups: string[] }, string>({
      query: (botId) => `/bots/${botId}/settings`,
      providesTags: (_r, _e, botId) => [{ type: 'Config', id: `${botId}:settings` }],
    }),
    putSettings: builder.mutation<BotConfig, { botId: string; version: number; values: Record<string, number> }>({
      query: ({ botId, ...body }) => ({ url: `/bots/${botId}/settings`, method: 'PUT', body }),
      invalidatesTags: (_r, _e, { botId }) => [
        { type: 'Config', id: `${botId}:settings` },
        { type: 'Config', id: botId },
      ],
    }),
    listActiveChats: builder.query<{ chats: ActiveChat[]; total: number }, string>({
      query: (botId) => `/bots/${botId}/admin/chats`,
      providesTags: (_r, _e, botId) => [{ type: 'Chats', id: `${botId}:active` }],
    }),
    getChatTranscript: builder.query<{ messages: TranscriptEntry[]; total: number }, { botId: string; a: number; b: number }>({
      query: ({ botId, a, b }) => `/bots/${botId}/admin/chats/transcript?a=${a}&b=${b}`,
      providesTags: (_r, _e, { botId }) => [{ type: 'Chats', id: `${botId}:transcript` }],
    }),
    listRecentChatMessages: builder.query<{ messages: ChatPreview[]; total: number }, string>({
      query: (botId) => `/bots/${botId}/admin/messages?limit=100`,
      providesTags: (_r, _e, botId) => [{ type: 'Chats', id: `${botId}:recent` }],
    }),
    getChannels: builder.query<{ channels: ChannelEntry[] }, string>({
      query: (botId) => `/bots/${botId}/admin/channels`,
      providesTags: (_r, _e, botId) => [{ type: 'Channels', id: botId }],
    }),
    putChannels: builder.mutation<BotConfig, { botId: string; version: number; channels: ChannelEntry[] }>({
      query: ({ botId, ...body }) => ({ url: `/bots/${botId}/admin/channels`, method: 'PUT', body }),
      invalidatesTags: (_r, _e, { botId }) => [
        { type: 'Channels', id: botId },
        { type: 'Config', id: botId },
      ],
    }),
    getAI: builder.query<AIHarness, string>({
      query: (botId) => `/bots/${botId}/admin/ai`,
      providesTags: (_r, _e, botId) => [{ type: 'AI', id: botId }],
    }),
    getAIDetail: builder.query<AIDetail, { botId: string; userId: number }>({
      query: ({ botId, userId }) => `/bots/${botId}/admin/ai/users/${userId}`,
    }),
    setAIEnabled: builder.mutation<BotConfig, { botId: string; enabled: boolean }>({
      query: ({ botId, enabled }) => ({
        url: `/bots/${botId}/admin/ai`,
        method: 'PUT',
        body: { enabled },
      }),
      invalidatesTags: (_r, _e, { botId }) => [
        { type: 'AI', id: botId },
        { type: 'Config', id: botId },
        { type: 'Config', id: `${botId}:settings` },
      ],
    }),
    scanTelegramBlocks: builder.mutation<ScanResult, { botId: string; days: number }>({
      query: ({ botId, days }) => ({
        url: `/bots/${botId}/admin/telegram-blocks/scan`,
        method: 'POST',
        body: { days },
      }),
      invalidatesTags: (_r, _e, { botId }) => [
        { type: 'BotUser', botId },
        { type: 'AdminOps', id: `${botId}:stuck` },
      ],
    }),
    listBlockPairs: builder.query<{ blocks: BlockPair[]; total: number }, string>({
      query: (botId) => `/bots/${botId}/admin/blocks`,
      providesTags: (_r, _e, botId) => [{ type: 'Blocks', id: botId }],
    }),
  }),
})

export const {
  useLoginMutation,
  useMeQuery,
  useListBotsQuery,
  useCreateBotMutation,
  useCreateBotFromAccountMutation,
  useGetBotQuery,
  useDeleteBotMutation,
  useStartBotMutation,
  useStopBotMutation,
  useGetBotStatsQuery,
  useListBotUsersQuery,
  useListEventsQuery,
  useListPaymentsQuery,
  useCreatePaymentMutation,
  useGetConfigQuery,
  usePutConfigMutation,
  useGetContentSettingsQuery,
  usePutContentSettingsMutation,
  useGetContentFoldersQuery,
  useGetContentChatsQuery,
  useGetContentPostsQuery,
  useCloneBotMutation,
  useFinanceOverviewQuery,
  useListAdminsQuery,
  useCreateAdminMutation,
  useDeleteAdminMutation,
  useSetBotAccessMutation,
  useListConfessionsQuery,
  useApproveConfessionMutation,
  useRejectConfessionMutation,
  useSeedConfessionsMutation,
  useListTicketsQuery,
  useReplyTicketMutation,
  useCloseTicketMutation,
  useListDatingUsersQuery,
  useBroadcastPreviewQuery,
  useStartBroadcastMutation,
  useListBroadcastsQuery,
  useListDiscountsQuery,
  useCreateDiscountMutation,
  useDeleteDiscountMutation,
  useGetUserDetailQuery,
  useAdjustCoinsMutation,
  useResetUserMutation,
  useSetUserBlockedMutation,
  useResetStreakMutation,
  useDirectMessageMutation,
  useListStuckUsersQuery,
  useResetStuckMutation,
  useResetAllChatsMutation,
  useGetSettingsQuery,
  usePutSettingsMutation,
  useListActiveChatsQuery,
  useGetChatTranscriptQuery,
  useListRecentChatMessagesQuery,
  useGetChannelsQuery,
  usePutChannelsMutation,
  useGetAIQuery,
  useGetAIDetailQuery,
  useSetAIEnabledMutation,
  useScanTelegramBlocksMutation,
  useListBlockPairsQuery,
} = sadbotApi
