import { afterEach, describe, expect, it, vi } from 'vitest'

describe('formulas api', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.resetModules()
  })

  it('maps list paging params to the backend limit/offset query', async () => {
    const fetchMock = vi.fn(async (input: string, init?: RequestInit) => {
      void input
      void init
      return {
        ok: true,
        status: 200,
        json: async () => ({ formulas: [], total: 0 }),
      }
    })
    vi.stubGlobal('fetch', fetchMock)

    const { listFormulas } = await import('./formulas')

    await listFormulas({ page: 3, pageSize: 25, search: 'reserve', domain: 'life' })

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/formulas?domain=life&search=reserve&limit=25&offset=50',
      expect.objectContaining({
        method: 'GET',
        credentials: 'include',
      }),
    )
  })
})
