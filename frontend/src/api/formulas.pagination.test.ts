import { describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({ get: vi.fn() }))

vi.mock('./client', () => ({ api: { get: mocks.get } }))

import { listAllFormulaIds } from './formulas'

describe('listAllFormulaIds', () => {
  it('requests every page, so export-all includes more than 500 matching formulas', async () => {
    mocks.get
      .mockResolvedValueOnce({ formulas: Array.from({ length: 500 }, (_, index) => ({ id: `f-${index}` })), total: 501 })
      .mockResolvedValueOnce({ formulas: [{ id: 'f-500' }], total: 501 })

    await expect(listAllFormulaIds({ search: 'premium' })).resolves.toHaveLength(501)
    expect(mocks.get).toHaveBeenNthCalledWith(1, '/formulas?search=premium&limit=500&offset=0')
    expect(mocks.get).toHaveBeenNthCalledWith(2, '/formulas?search=premium&limit=500&offset=500')
  })
})
