import type { ReactNode } from 'react'
import { Navigate, useLocation } from 'react-router-dom'

export default function RequireAuth({ children }: { children: ReactNode }) {
  const token = localStorage.getItem('sadbot_token')
  const location = useLocation()
  if (!token) {
    return <Navigate to="/login" state={{ from: location }} replace />
  }
  return <>{children}</>
}
