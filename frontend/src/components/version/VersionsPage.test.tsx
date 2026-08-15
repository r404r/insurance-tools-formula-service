// @vitest-environment jsdom

import type { ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'

const mocks = vi.hoisted(() => ({
  user: { id: 'viewer-1', role: 'viewer' },
  mutate: vi.fn(),
}))

vi.mock('react-router-dom', () => ({
  Link: ({ children }: { children: ReactNode }) => <a>{children}</a>,
  useNavigate: () => vi.fn(),
  useParams: () => ({ id: 'formula-1' }),
}))
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }))
vi.mock('../../store/authStore', () => ({ useAuthStore: (selector: (state: unknown) => unknown) => selector({ user: mocks.user }) }))
vi.mock('@tanstack/react-query', () => ({
  useQuery: ({ queryKey }: { queryKey: string[] }) => queryKey[0] === 'formula'
    ? { data: { id: 'formula-1', name: 'Premium' } }
    : { data: [{ version: 1, state: 'draft', createdAt: '2026-08-15T00:00:00Z' }], isLoading: false },
  useMutation: () => ({ mutate: mocks.mutate, isError: true, error: new Error('publish rejected') }),
  useQueryClient: () => ({ invalidateQueries: vi.fn() }),
}))
vi.mock('../../api/client', () => ({ api: { get: vi.fn(), patch: vi.fn() } }))
vi.mock('./VersionDiffModal', () => ({ default: () => null }))

import VersionsPage from './VersionsPage'

describe('VersionsPage permissions and mutation feedback', () => {
  it('does not offer publish or archive controls to a viewer and makes a mutation failure visible', () => {
    render(<VersionsPage />)

    expect(screen.queryByRole('button', { name: 'version.publish' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'version.archive' })).toBeNull()
    expect(screen.getByText('publish rejected')).toBeTruthy()
  })
})
