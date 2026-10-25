import { createBrowserRouter, Navigate } from 'react-router-dom'
import AdminLayout from '../layouts/AdminLayout'
import RequireAuth from '../features/auth/RequireAuth'
import LoginPage from '../features/auth/LoginPage'
import FinancePage from '../features/finance/FinancePage'
import BotsPage from '../features/bots/BotsPage'
import BotDetailPage from '../features/bots/BotDetailPage'
import AdminsPage from '../features/admins/AdminsPage'
import AccountsPage from '../features/telegram/AccountsPage'
import ChatViewPage from '../features/telegram/ChatViewPage'

export const router = createBrowserRouter([
  { path: '/login', element: <LoginPage /> },
  {
    path: '/',
    element: (
      <RequireAuth>
        <AdminLayout />
      </RequireAuth>
    ),
    children: [
      { index: true, element: <FinancePage /> },
      { path: 'bots', element: <BotsPage /> },
      { path: 'bots/:id', element: <BotDetailPage /> },
      { path: 'admins', element: <AdminsPage /> },
      { path: 'telegram', element: <AccountsPage /> },
      { path: 'telegram/:accountId', element: <ChatViewPage /> },
      { path: '*', element: <Navigate to="/" replace /> },
    ],
  },
])
