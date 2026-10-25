import { sadbotApi } from '../bots/botsApi'

export { sadbotApi }

export interface TgAccount {
  id: string
  admin_id: string
  label: string
  tg_user_id: number
  phone: string
  sidecar_account_id: string
  created_at: string
  display_name?: string
  username?: string
}

export interface TgChat {
  chat_id: string
  title: string
  type: 'user' | 'group' | 'channel' | 'bot' | string
  unread: number
  pinned: boolean
  last_message?: { text: string; date?: string | null } | null
}

export interface TgSender {
  id: string
  name: string
  is_out: boolean
}

export interface TgMessage {
  id: number
  date?: string | null
  sender: TgSender
  text: string
  media: { kind: string }
  reply_to_id?: number | null
}

export interface TgMessagesPage {
  messages: TgMessage[]
  oldest_id: number
}

export const telegramApi = sadbotApi.injectEndpoints({
  endpoints: (build) => ({
    tgStartLogin: build.mutation<{ ok: boolean; pending_id: string }, { phone: string }>({
      query: (body) => ({ url: '/telegram-accounts/start-login', method: 'POST', body }),
    }),
    tgCompleteLogin: build.mutation<
      TgAccount,
      { pending_id: string; phone: string; code: string; password?: string; label?: string }
    >({
      query: (body) => ({ url: '/telegram-accounts/complete-login', method: 'POST', body }),
      invalidatesTags: [{ type: 'TgAccount' as const, id: 'LIST' }],
    }),
    tgListAccounts: build.query<{ accounts: TgAccount[] }, void>({
      query: () => '/telegram-accounts',
      providesTags: (result) =>
        // guard the array too: an empty/error body without `accounts` must not crash
        result?.accounts
          ? [...result.accounts.map((a) => ({ type: 'TgAccount' as const, id: a.id })), { type: 'TgAccount' as const, id: 'LIST' }]
          : [{ type: 'TgAccount' as const, id: 'LIST' }],
    }),
    tgDeleteAccount: build.mutation<{ ok: boolean }, string>({
      query: (id) => ({ url: `/telegram-accounts/${id}`, method: 'DELETE' }),
      invalidatesTags: [{ type: 'TgAccount' as const, id: 'LIST' }],
    }),
    tgGetChats: build.query<{ chats: TgChat[] }, { accountId: string }>({
      query: ({ accountId }) => `/telegram-accounts/${accountId}/chats`,
      providesTags: (_r, _e, { accountId }) => [{ type: 'TgChat' as const, id: accountId }],
    }),
    tgGetMessages: build.query<TgMessagesPage, { accountId: string; chatId: string; offsetId?: number }>({
      query: ({ accountId, chatId, offsetId }) => ({
        url: `/telegram-accounts/${accountId}/chats/${chatId}/messages`,
        params: { limit: 50, offset_id: offsetId ?? 0 },
      }),
      // all pages of one chat share a single cache entry; polling replaces,
      // "load older" prepends.
      serializeQueryArgs: ({ queryArgs }) => `tgMsgs:${queryArgs.accountId}:${queryArgs.chatId}`,
      merge: (current, incoming, { arg }) => {
        if (arg.offsetId) {
          const known = new Set(current.messages.map((m) => m.id))
          const older = incoming.messages.filter((m) => !known.has(m.id))
          return { messages: [...older, ...current.messages], oldest_id: incoming.oldest_id }
        }
        return incoming
      },
      forceRefetch: ({ currentArg, previousArg }) =>
        currentArg?.offsetId !== previousArg?.offsetId,
      providesTags: (_r, _e, { accountId, chatId }) => [{ type: 'TgChat' as const, id: `${accountId}:${chatId}` }],
    }),
    tgSendMessage: build.mutation<{ message: TgMessage }, { accountId: string; chatId: string; text: string }>({
      query: ({ accountId, chatId, text }) => ({
        url: `/telegram-accounts/${accountId}/chats/${chatId}/messages`,
        method: 'POST',
        body: { text },
      }),
      // refresh this chat's messages and the chat list (last_message changes)
      invalidatesTags: (_r, _e, { accountId, chatId }) => [
        { type: 'TgChat' as const, id: `${accountId}:${chatId}` },
        { type: 'TgChat' as const, id: accountId },
      ],
    }),
  }),
})

export const {
  useTgStartLoginMutation,
  useTgCompleteLoginMutation,
  useTgListAccountsQuery,
  useTgDeleteAccountMutation,
  useTgGetChatsQuery,
  useTgGetMessagesQuery,
  useTgSendMessageMutation,
} = telegramApi

export function tgPhotoUrl(accountId: string, chatId: string, msgId: number): string {
  return `/api/v1/telegram-accounts/${accountId}/chats/${chatId}/messages/${msgId}/photo`
}

export function tgChatPhotoUrl(accountId: string, chatId: string): string {
  return `/api/v1/telegram-accounts/${accountId}/chats/${chatId}/photo`
}
