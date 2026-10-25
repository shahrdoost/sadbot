import { configureStore } from '@reduxjs/toolkit'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Provider } from 'react-redux'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { jsonResponse, stubFetch } from '../../test/stubFetch'
import { sadbotApi } from './botsApi'
import BotsPage from './BotsPage'

function renderBots() {
  const store = configureStore({
    reducer: { [sadbotApi.reducerPath]: sadbotApi.reducer },
    middleware: (getDefault) => getDefault().concat(sadbotApi.middleware),
  })
  render(
    <Provider store={store}>
      <MemoryRouter>
        <BotsPage />
      </MemoryRouter>
    </Provider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('BotsPage create form', () => {
  it('defaults to account mode and shows the accounts dropdown', async () => {
    stubFetch((url) => {
      if (url.includes('/telegram-accounts')) return jsonResponse({ accounts: [] })
      return jsonResponse({ bots: [] })
    })
    renderBots()
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: /ساخت بات جدید/ }))

    expect(screen.getByText('از اکانت تلگرام')).toBeInTheDocument()
    expect(screen.getByLabelText(/کدام اکانت/)).toBeInTheDocument()
    expect(screen.getByText(/اکانتی اضافه نکرده‌اید/)).toBeInTheDocument()
    // token field must be hidden in account mode
    expect(screen.queryByLabelText('توکن ربات تلگرام')).not.toBeInTheDocument()
  })

  it('switches to token mode and reveals the token input', async () => {
    stubFetch((url) => {
      if (url.includes('/telegram-accounts')) return jsonResponse({ accounts: [] })
      return jsonResponse({ bots: [] })
    })
    renderBots()
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: /ساخت بات جدید/ }))
    await user.click(screen.getByRole('button', { name: 'با توکن' }))

    expect(screen.getByLabelText('توکن ربات تلگرام')).toBeInTheDocument()
    expect(screen.queryByLabelText(/کدام اکانت/)).not.toBeInTheDocument()
  })

  it('lists account options and posts to create-bot when submitted', async () => {
    const calls: { url: string; body: unknown }[] = []
    stubFetch((url, init) => {
      if (url.includes('/telegram-accounts') && !url.includes('create-bot')) {
        return jsonResponse({
          accounts: [
            {
              id: 'acc1',
              admin_id: 'x',
              label: '',
              tg_user_id: 1,
              phone: '+9890',
              sidecar_account_id: 's1',
              created_at: '2026-10-14T10:00:00Z',
              display_name: 'Dastan',
            },
          ],
        })
      }
      if (url.endsWith('/create-bot')) {
        calls.push({ url, body: init?.body })
        return jsonResponse({ id: 'bot1', name: 'بات تست', template_key: 'welcome_menu' }, 201)
      }
      return jsonResponse({ bots: [] })
    })
    renderBots()
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: /ساخت بات جدید/ }))

    await user.type(screen.getByLabelText('نام بات'), 'بات تست')
    await user.click(screen.getByRole('button', { name: 'ساخت بات' }))

    await vi.waitFor(() => expect(calls).toHaveLength(1))
    expect(calls[0].url).toContain('/telegram-accounts/acc1/create-bot')
    expect(String(calls[0].body)).toContain('بات تست')
  })
})
