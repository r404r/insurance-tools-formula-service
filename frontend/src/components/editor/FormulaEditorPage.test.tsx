// @vitest-environment jsdom

import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import FormulaEditorPage from './FormulaEditorPage'

const mocks = vi.hoisted(() => ({
  post: vi.fn().mockResolvedValue({ result: {} }),
  invalidateQueries: vi.fn(),
  navigate: vi.fn(),
  formula: {
    id: 'formula-1', name: 'Premium', domain: 'life', description: '',
    createdBy: 'user-1', createdAt: '2026-08-15T00:00:00Z', updatedAt: '2026-08-15T00:00:00Z',
  },
  versions: [{
    id: 'version-1', formulaId: 'formula-1', version: 1, state: 'draft',
    graph: {
      nodes: [{ id: 'age', type: 'variable', config: { name: 'age', dataType: 'integer' } }],
      edges: [], outputs: ['age'],
    },
    changeNote: '', createdBy: 'user-1', createdAt: '2026-08-15T00:00:00Z',
  }],
  empty: [] as unknown[],
  allFormulas: [] as Array<{ id: string; name: string }>,
}))

vi.mock('react-router-dom', () => ({
  Link: ({ children }: { children: ReactNode }) => <a>{children}</a>,
  useNavigate: () => mocks.navigate,
  useParams: () => ({ id: 'formula-1' }),
  useSearchParams: () => [new URLSearchParams(), vi.fn()],
}))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

vi.mock('@tanstack/react-query', () => ({
  useQuery: ({ queryKey }: { queryKey: string[] }) => {
    if (queryKey[0] === 'formula') {
      return {
        data: mocks.formula,
      }
    }
    if (queryKey[0] === 'versions') {
      return {
        data: mocks.versions,
      }
    }
    if (queryKey[0] === 'formulas') {
      return { data: mocks.allFormulas }
    }
    return { data: mocks.empty }
  },
  useQueryClient: () => ({ invalidateQueries: mocks.invalidateQueries }),
}))

vi.mock('../../api/client', () => ({
  api: { get: vi.fn(), post: mocks.post, put: vi.fn() },
}))

vi.mock('../../api/categories', () => ({ listCategories: vi.fn() }))
vi.mock('./FormulaCanvas', () => ({
  default: ({ nodes, onNodesChange }: { nodes: Array<{ id: string }>; onNodesChange: (next: Array<{ id: string }>) => void }) => (
    <div>
      <output data-testid="formula-canvas">{nodes.map((node) => node.id).join(',')}</output>
      <button onClick={() => onNodesChange([{ id: 'user-edit' }])}>simulate graph edit</button>
    </div>
  ),
}))
vi.mock('./NodePalette', () => ({ default: () => <div data-testid="node-palette" /> }))
vi.mock('./NodePropertiesPanel', () => ({ default: () => <div data-testid="node-properties" /> }))
vi.mock('./TextEditor', () => ({ default: () => <div data-testid="text-editor" /> }))
vi.mock('./hooks/useAutoLayout', () => ({ useAutoLayout: () => (nodes: unknown[]) => nodes }))

afterEach(() => {
  cleanup()
  mocks.post.mockClear()
  mocks.allFormulas = []
})

describe('FormulaEditorPage test input validation', () => {
  it('does not submit the prior valid inputs after the currently displayed JSON becomes invalid', () => {
    render(<FormulaEditorPage />)

    const input = screen.getByPlaceholderText(/calc\.inputs/) as HTMLInputElement
    fireEvent.change(input, { target: { value: '{"age":"35"}' } })
    fireEvent.change(input, { target: { value: '{"age":' } })
    fireEvent.click(screen.getByRole('button', { name: 'calc.calculate' }))

    expect(mocks.post).not.toHaveBeenCalled()
  })

  it('does not replace the locally edited graph when formula-name enrichment arrives later', async () => {
    const view = render(<FormulaEditorPage />)

    await waitFor(() => {
      expect(screen.getByTestId('formula-canvas').textContent).toBe('age')
    })
    fireEvent.click(screen.getByRole('button', { name: 'simulate graph edit' }))
    expect(screen.getByTestId('formula-canvas').textContent).toBe('user-edit')

    mocks.allFormulas = [{ id: 'formula-ref', name: 'Referenced formula' }]
    view.rerender(<FormulaEditorPage />)

    expect(screen.getByTestId('formula-canvas').textContent).toBe('user-edit')
  })
})
