import { afterEach, describe, expect, it, vi } from 'vitest'
import type { User } from '../types/formula'

const user: User = {
  id: 'u1',
  username: 'alice',
  role: 'admin',
  createdAt: '2026-01-01T00:00:00Z',
}

describe('auth store cookie auth state', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.resetModules()
  })

  it('does not derive authentication from localStorage tokens', async () => {
    const legacyStorage = {
      getItem: vi.fn(() => 'legacy-token'),
      setItem: vi.fn(),
      removeItem: vi.fn(),
    }
    vi.stubGlobal('localStorage', legacyStorage)

    const { useAuthStore } = await import('./authStore')
    const state = useAuthStore.getState()

    expect(legacyStorage.getItem).not.toHaveBeenCalled()
    expect(state.user).toBeNull()
    expect(state.isAuthenticated).toBe(false)
    expect(state.isAuthChecked).toBe(false)
    expect('token' in state).toBe(false)
  })

  it('marks a user authenticated without persisting a token', async () => {
    const legacyStorage = {
      getItem: vi.fn(() => null),
      setItem: vi.fn(),
      removeItem: vi.fn(),
    }
    vi.stubGlobal('localStorage', legacyStorage)

    const { useAuthStore } = await import('./authStore')

    useAuthStore.getState().login(user)

    expect(legacyStorage.setItem).not.toHaveBeenCalled()
    expect(legacyStorage.removeItem).not.toHaveBeenCalled()
    expect(useAuthStore.getState()).toMatchObject({
      user,
      isAuthenticated: true,
      isAuthChecked: true,
    })
  })
})
