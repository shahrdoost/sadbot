import { configureStore } from '@reduxjs/toolkit'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Provider } from 'react-redux'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { jsonResponse, stubFetch } from '../../test/stubFetch'
import { sadbotApi } from '../bots/botsApi'
import AccountsPage from './AccountsPage'

function renderPage() {
  const store = configureStore({
    reducer: { [sadbotApi.reducerPath]: sadbotApi.reducer },
    middleware: (getDefault) => getDefault().concat(sadbotApi.middleware),
  })
  render(
    <Provider store={store}>
      <MemoryRouter>
        <AccountsPage />
      </MemoryRouter>
    </Provider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('AccountsPage', () => {
  it('shows empty state when no accounts', async () => {
    stubFetch((url) => {
      if (url.includes('/telegram-accounts')) return jsonResponse({ accounts: [] })
      return jsonResponse({})
    })
    renderPage()
    expect(await screen.findByText(/هنوز اکانتی اضافه نکرده‌اید/)).toBeInTheDocument()
  })

  it('lists accounts with username', async () => {
    stubFetch((url) => {
      if (url.includes('/telegram-accounts'))
        return jsonResponse({
          accounts: [
            {
              id: 'a1',
              admin_id: 'x',
              label: '',
              tg_user_id: 1,
              phone: '+989039816504',
              sidecar_account_id: 's1',
              created_at: '2026-10-14T10:00:00Z',
              display_name: 'Dastan',
              username: 'dastan_group_com',
            },
          ],
        })
      return jsonResponse({})
    })
    renderPage()
    expect(await screen.findByText('Dastan')).toBeInTheDocument()
    expect(screen.getByText('@dastan_group_com')).toBeInTheDocument()
  })

  it('add wizard: phone → code step', async () => {
    stubFetch((url, _init) => {
      if (url.endsWith('/start-login')) return jsonResponse({ ok: true, pending_id: 'p1' })
      if (url.includes('/telegram-accounts')) return jsonResponse({ accounts: [] })
      return jsonResponse({})
    })
    renderPage()
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: /افزودن اکانت/ }))

    await user.type(screen.getByLabelText(/شماره تلفن/), '+989039816504')
    await user.click(screen.getByRole('button', { name: /ارسال کد تایید/ }))

    expect(await screen.findByText(/کد تایید به شماره/)).toBeInTheDocument()
    expect(screen.getByLabelText('کد تایید')).toBeInTheDocument()
  })

  it('add wizard: PASSWORD_NEEDED switches to password step', async () => {
    let loginCalls = 0
    stubFetch((url) => {
      if (url.endsWith('/start-login')) return jsonResponse({ ok: true, pending_id: 'p1' })
      if (url.endsWith('/complete-login')) {
        loginCalls++
        return jsonResponse({ error: 'PASSWORD_NEEDED' }, 401)
      }
      if (url.includes('/telegram-accounts')) return jsonResponse({ accounts: [] })
      return jsonResponse({})
    })
    renderPage()
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: /افزودن اکانت/ }))
    await user.type(screen.getByLabelText(/شماره تلفن/), '+989039816504')
    await user.click(screen.getByRole('button', { name: /ارسال کد تایید/ }))
    await user.type(await screen.findByLabelText('کد تایید'), '12345')
    await user.click(screen.getByRole('button', { name: /^ورود$/ }))

    expect(await screen.findByLabelText('رمز تایید دو مرحله‌ای')).toBeInTheDocument()
    expect(loginCalls).toBe(1)
  })
})
