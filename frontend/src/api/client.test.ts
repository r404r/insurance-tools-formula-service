import { afterEach, describe, expect, it, vi } from 'vitest'

describe('api client cookie auth', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.resetModules()
  })

  it('uses browser credentials without reading or sending legacy bearer tokens', async () => {
    const legacyStorage = {
      getItem: vi.fn(() => 'legacy-token'),
      setItem: vi.fn(),
      removeItem: vi.fn(),
    }
    const fetchMock = vi.fn(async (input: string, init?: RequestInit) => {
      void input
      void init
      return {
        ok: true,
        status: 200,
        json: async () => ({ id: 'u1' }),
      }
    })

    vi.stubGlobal('localStorage', legacyStorage)
    vi.stubGlobal('fetch', fetchMock)

    const { api } = await import('./client')

    await api.get('/auth/me')

    expect(legacyStorage.getItem).not.toHaveBeenCalled()
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/auth/me',
      expect.objectContaining({
        method: 'GET',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
      }),
    )
    const requestInit = fetchMock.mock.calls[0]?.[1]
    expect(requestInit).not.toHaveProperty('headers.Authorization')
  })
})
