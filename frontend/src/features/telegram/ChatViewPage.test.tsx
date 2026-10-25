import { configureStore } from '@reduxjs/toolkit'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Provider } from 'react-redux'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { jsonResponse, stubFetch } from '../../test/stubFetch'
import { sadbotApi } from '../bots/botsApi'
import ChatViewPage from './ChatViewPage'

function renderChat() {
  const store = configureStore({
    reducer: { [sadbotApi.reducerPath]: sadbotApi.reducer },
    middleware: (getDefault) => getDefault().concat(sadbotApi.middleware),
  })
  render(
    <Provider store={store}>
      <MemoryRouter initialEntries={['/telegram/acc1']}>
        <Routes>
          <Route path="/telegram/:accountId" element={<ChatViewPage />} />
        </Routes>
      </MemoryRouter>
    </Provider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('ChatViewPage', () => {
  it('shows chat list and renders outgoing bubbles on the visual left (justify-end in RTL)', async () => {
    stubFetch((url) => {
      if (url.includes('/chats') && !url.includes('/messages'))
        return jsonResponse({
          chats: [
            {
              chat_id: '-1001',
              title: 'کانال تست',
              type: 'channel',
              unread: 2,
              pinned: false,
              last_message: { text: 'سلام دنیا', date: '2026-10-14T10:00:00Z' },
            },
          ],
        })
      if (url.includes('/messages'))
        return jsonResponse({
          messages: [
            { id: 1, date: '2026-10-14T10:00:00Z', sender: { id: '9', name: 'کانال تست', is_out: false }, text: 'پیام کانال', media: { kind: 'none' } },
            { id: 2, date: '2026-14-14T10:01:00Z', sender: { id: '1', name: 'من', is_out: true }, text: 'پاسخ من', media: { kind: 'none' } },
          ],
          oldest_id: 1,
        })
      return jsonResponse({})
    })
    renderChat()
    const user = userEvent.setup()

    await user.click(await screen.findByText('کانال تست'))
    expect(await screen.findByText('پیام کانال')).toBeInTheDocument()
    expect(screen.getByText('پاسخ من')).toBeInTheDocument()

    const outBubble = screen.getByText('پاسخ من').closest('[data-mine]') as HTMLElement
    const inBubble = screen.getByText('پیام کانال').closest('[data-mine]') as HTMLElement
    expect(outBubble.className).toContain('justify-end')
    expect(inBubble.className).toContain('justify-start')
  })

  it('renders photo placeholder for photo messages', async () => {
    stubFetch((url) => {
      if (url.includes('/messages/') && url.endsWith('/photo')) return jsonResponse({ error: 'no' }, 404)
      if (url.includes('/chats') && !url.includes('/messages'))
        return jsonResponse({ chats: [{ chat_id: '-1002', title: 'آپلود فایل', type: 'channel', unread: 0, pinned: false }] })
      if (url.includes('/messages'))
        return jsonResponse({
          messages: [{ id: 5, date: null, sender: { id: '9', name: '', is_out: false }, text: '', media: { kind: 'photo' } }],
          oldest_id: 5,
        })
      return jsonResponse({})
    })
    renderChat()
    const user = userEvent.setup()
    await user.click(await screen.findByText('آپلود فایل'))
    expect(await screen.findByText(/عکس بارگذاری نشد|در حال بارگذاری عکس/)).toBeInTheDocument()
  })

  it('sends a message from the composer via POST', async () => {
    const posts: { url: string; body: string }[] = []
    stubFetch((url, init) => {
      if (init?.method === 'POST' && url.includes('/messages')) {
        posts.push({ url, body: String(init.body) })
        return jsonResponse({
          message: { id: 3, text: 'پیام جدید', sender: { id: '1', name: 'من', is_out: true }, media: { kind: 'none' } },
        })
      }
      if (url.includes('/chats') && !url.includes('/messages'))
        return jsonResponse({ chats: [{ chat_id: '-1001', title: 'کانال تست', type: 'channel', unread: 0, pinned: false }] })
      if (url.includes('/messages'))
        return jsonResponse({ messages: [], oldest_id: 0 })
      return jsonResponse({})
    })
    renderChat()
    const user = userEvent.setup()

    await user.click(await screen.findByText('کانال تست'))
    const input = await screen.findByPlaceholderText(/پیام بنویسید/)
    await user.type(input, 'سلام تازه')
    await user.click(screen.getByRole('button', { name: 'ارسال' }))

    await vi.waitFor(() => {
      expect(posts).toHaveLength(1)
    })
    expect(posts[0].url).toContain('/telegram-accounts/acc1/chats/-1001/messages')
    expect(JSON.parse(posts[0].body)).toEqual({ text: 'سلام تازه' })
    await vi.waitFor(() => {
      expect(screen.getByPlaceholderText(/پیام بنویسید/)).toHaveValue('')
    })
  })

  it('shows a Persian error when send is rejected', async () => {
    stubFetch((url, init) => {
      if (init?.method === 'POST' && url.includes('/messages'))
        return jsonResponse({ error: 'امکان ارسال پیام در این چت را ندارید' }, 403)
      if (url.includes('/chats') && !url.includes('/messages'))
        return jsonResponse({ chats: [{ chat_id: '-1001', title: 'کانال تست', type: 'channel', unread: 0, pinned: false }] })
      if (url.includes('/messages'))
        return jsonResponse({ messages: [], oldest_id: 0 })
      return jsonResponse({})
    })
    renderChat()
    const user = userEvent.setup()

    await user.click(await screen.findByText('کانال تست'))
    const input = await screen.findByPlaceholderText(/پیام بنویسید/)
    await user.type(input, 'تست خطا')
    await user.click(screen.getByRole('button', { name: 'ارسال' }))

    expect(await screen.findByText('امکان ارسال پیام در این چت را ندارید')).toBeInTheDocument()
    // text stays in the box for retry
    expect(screen.getByPlaceholderText(/پیام بنویسید/)).toHaveValue('تست خطا')
  })
})
