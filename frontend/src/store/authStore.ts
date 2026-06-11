import { create } from 'zustand'
import type { User } from '../types/formula'

interface AuthState {
  user: User | null
  isAuthenticated: boolean
  isAuthChecked: boolean
  login: (user: User) => void
  logout: () => void
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  isAuthenticated: false,
  isAuthChecked: false,
  login: (user) => {
    set({ user, isAuthenticated: true, isAuthChecked: true })
  },
  logout: () => {
    set({ user: null, isAuthenticated: false, isAuthChecked: true })
  },
}))
