import { describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({ post: vi.fn() }))

vi.mock('./client', () => ({ api: { get: vi.fn(), post: mocks.post } }))

import { instantiateTemplate } from './templates'

describe('instantiateTemplate', () => {
  it('uses the single atomic template-instantiation endpoint', async () => {
    const formula = { id: 'formula-1', name: 'New formula' }
    mocks.post.mockResolvedValueOnce(formula)

    await expect(instantiateTemplate('template-1', {
      name: 'New formula', domain: 'life', description: 'from a template',
    })).resolves.toEqual(formula)

    expect(mocks.post).toHaveBeenCalledWith('/templates/template-1/instantiate', {
      name: 'New formula', domain: 'life', description: 'from a template',
    })
  })
})
