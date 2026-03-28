import { create } from 'zustand'
import type { User } from '../types/formula'
import { setToken } from '../api/client'

interface AuthState {
  user: User | null
  token: string | null
  isAuthenticated: boolean
  login: (token: string, user: User) => void
  logout: () => void
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  token: localStorage.getItem('token'),
  isAuthenticated: !!localStorage.getItem('token'),
  login: (token, user) => {
    setToken(token)
    set({ token, user, isAuthenticated: true })
  },
  logout: () => {
    setToken(null)
    set({ token: null, user: null, isAuthenticated: false })
  },
}))
