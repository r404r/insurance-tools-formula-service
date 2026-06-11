import { afterEach, describe, expect, it, vi } from 'vitest'

describe('auth api', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.resetModules()
  })

  it('posts logout through the cookie-auth client', async () => {
    const fetchMock = vi.fn(async (input: string, init?: RequestInit) => {
      void input
      void init
      return {
        ok: true,
        status: 204,
        json: async () => ({}),
      }
    })
    vi.stubGlobal('fetch', fetchMock)

    const auth = await import('./auth')

    expect(auth.logout).toBeTypeOf('function')
    await auth.logout()

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/auth/logout',
      expect.objectContaining({
        method: 'POST',
        credentials: 'include',
      }),
    )
  })
})
