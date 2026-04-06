import { useEffect } from 'react'
import { BrowserRouter, Routes, Route, Navigate, Outlet } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useAuthStore } from './store/authStore'
import { getMe } from './api/auth'
import LoginPage from './components/auth/LoginPage'
import RegisterPage from './components/auth/RegisterPage'
import Layout from './components/shared/Layout'
import FormulaList from './components/shared/FormulaList'
import CategoryManagementPage from './components/shared/CategoryManagementPage'
import UserManagementPage from './components/shared/UserManagementPage'
import FormulaEditorPage from './components/editor/FormulaEditorPage'
import VersionsPage from './components/version/VersionsPage'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      staleTime: 30_000,
    },
  },
})

function ProtectedRoute() {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)
  if (!isAuthenticated) return <Navigate to="/login" replace />
  return <Outlet />
}

function App() {
  const { isAuthenticated, user, login, logout } = useAuthStore()

  // Hydrate user from /auth/me on startup (token may survive page refresh but user object is lost).
  useEffect(() => {
    if (isAuthenticated && !user) {
      getMe()
        .then((me) => {
          const token = localStorage.getItem('token') ?? ''
          login(token, me)
        })
        .catch(() => logout())
    }
  }, [isAuthenticated, user, login, logout])

  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/register" element={<RegisterPage />} />
          <Route element={<ProtectedRoute />}>
            <Route element={<Layout />}>
              <Route path="/" element={<FormulaList />} />
              <Route path="/categories" element={<CategoryManagementPage />} />
              <Route path="/users" element={<UserManagementPage />} />
              <Route path="/formulas/:id/versions" element={<VersionsPage />} />
            </Route>
            <Route path="/formulas/:id" element={<FormulaEditorPage />} />
          </Route>
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  )
}

export default App
