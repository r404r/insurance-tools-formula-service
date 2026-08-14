import { beforeEach, describe, expect, it } from 'vitest'
import { useFormulaStore } from './formulaStore'
import type { FormulaGraph, FormulaVersion } from '../types/formula'

const serverGraph: FormulaGraph = {
  nodes: [{ id: 'server-input', type: 'variable', config: { name: 'age', dataType: 'integer' } }],
  edges: [],
  outputs: ['server-input'],
}

const userGraph: FormulaGraph = {
  nodes: [{ id: 'user-input', type: 'variable', config: { name: 'age', dataType: 'integer' } }],
  edges: [],
  outputs: ['user-input'],
}

function version(graph: FormulaGraph): FormulaVersion {
  return {
    id: 'version-42',
    formulaId: 'formula-42',
    version: 42,
    state: 'draft',
    graph,
    changeNote: 'Initial version',
    createdBy: 'user-1',
    createdAt: '2026-08-15T00:00:00Z',
  }
}

describe('formula hydration state', () => {
  beforeEach(() => {
    useFormulaStore.getState().reset()
  })

  it('does not overwrite a dirty user graph when the same version rehydrates for enrichment', () => {
    useFormulaStore.getState().setCurrentVersion(version(serverGraph))
    useFormulaStore.getState().setGraph(userGraph)

    // React Query can re-deliver the same version while the formula list is
    // fetched for display-name enrichment. It is not permission to replace
    // edits that have not yet been saved.
    useFormulaStore.getState().setCurrentVersion(version(serverGraph))

    expect(useFormulaStore.getState().graph).toEqual(userGraph)
    expect(useFormulaStore.getState().isDirty).toBe(true)
  })
})
