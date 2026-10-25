import { configureStore } from '@reduxjs/toolkit'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Provider } from 'react-redux'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { sadbotApi } from '../bots/botsApi'
import LoginPage from './LoginPage'

// Node's Request rejects the relative URLs that fetchBaseQuery produces in
// jsdom; resolve them against a fixed origin for the duration of each test.
function stubFetch(handler: (input: string) => Response) {
  const RealRequest = globalThis.Request
  class PatchedRequest extends RealRequest {
    constructor(input: RequestInfo | URL, init?: RequestInit) {
      if (typeof input === 'string' && input.startsWith('/')) {
        input = `http://localhost${input}`
      }
      super(input, init)
    }
  }
  vi.stubGlobal('Request', PatchedRequest)
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL) => {
      const url = typeof input === 'string' ? input : input instanceof Request ? input.url : String(input)
      return handler(url)
    }),
  )
}

function renderLogin() {
  const store = configureStore({
    reducer: { [sadbotApi.reducerPath]: sadbotApi.reducer },
    middleware: (getDefault) => getDefault().concat(sadbotApi.middleware),
  })
  render(
    <Provider store={store}>
      <MemoryRouter initialEntries={['/login']}>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/" element={<div>داشبورد</div>} />
        </Routes>
      </MemoryRouter>
    </Provider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('LoginPage', () => {
  it('renders the brand and form fields', () => {
    renderLogin()
    expect(screen.getByText('صدبات')).toBeInTheDocument()
    expect(screen.getByLabelText('ایمیل')).toBeInTheDocument()
    expect(screen.getByLabelText('رمز عبور')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /ورود/ })).toBeInTheDocument()
  })

  it('stores the token and navigates on successful login', async () => {
    stubFetch(
      () =>
        new Response(
          JSON.stringify({
            access_token: 'jwt-token-123',
            admin: { id: '1', email: 'admin@sadbot.local', role: 'superadmin' },
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        ),
    )

    renderLogin()
    const user = userEvent.setup()
    await user.type(screen.getByLabelText('ایمیل'), 'admin@sadbot.local')
    await user.type(screen.getByLabelText('رمز عبور'), 'password123')
    await user.click(screen.getByRole('button', { name: /ورود/ }))

    expect(await screen.findByText('داشبورد')).toBeInTheDocument()
    expect(localStorage.getItem('sadbot_token')).toBe('jwt-token-123')
  })

  it('shows the server error message on failed login', async () => {
    stubFetch(
      () =>
        new Response(JSON.stringify({ error: 'ایمیل یا رمز عبور نادرست است' }), {
          status: 401,
          headers: { 'Content-Type': 'application/json' },
        }),
    )

    renderLogin()
    const user = userEvent.setup()
    await user.type(screen.getByLabelText('ایمیل'), 'x@example.com')
    await user.type(screen.getByLabelText('رمز عبور'), 'wrongpass')
    await user.click(screen.getByRole('button', { name: /ورود/ }))

    expect(await screen.findByText('ایمیل یا رمز عبور نادرست است')).toBeInTheDocument()
    expect(localStorage.getItem('sadbot_token')).toBeNull()
  })
})
