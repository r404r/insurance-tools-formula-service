import { create } from 'zustand'
import type { Formula, FormulaVersion, FormulaGraph } from '../types/formula'

export type EditorMode = 'visual' | 'text'

interface FormulaState {
  currentFormula: Formula | null
  currentVersion: FormulaVersion | null
  graph: FormulaGraph | null
  editorMode: EditorMode
  isDirty: boolean
  setCurrentFormula: (formula: Formula | null) => void
  setCurrentVersion: (version: FormulaVersion | null) => void
  setGraph: (graph: FormulaGraph) => void
  setEditorMode: (mode: EditorMode) => void
  markDirty: () => void
  markClean: () => void
  reset: () => void
}

const initialState = {
  currentFormula: null,
  currentVersion: null,
  graph: null,
  editorMode: 'visual' as EditorMode,
  isDirty: false,
}

export const useFormulaStore = create<FormulaState>((set) => ({
  ...initialState,
  setCurrentFormula: (formula) => set({ currentFormula: formula }),
  setCurrentVersion: (version) =>
    set((state) => {
      // React Query may re-deliver the same version while auxiliary editor
      // data (such as formula names) is enriched. That is metadata refresh,
      // not permission to discard unsaved work.
      if (version && state.isDirty && state.currentVersion?.id === version.id) {
        return { currentVersion: version }
      }
      return { currentVersion: version, graph: version?.graph ?? null, isDirty: false }
    }),
  setGraph: (graph) => set({ graph, isDirty: true }),
  setEditorMode: (mode) => set({ editorMode: mode }),
  markDirty: () => set({ isDirty: true }),
  markClean: () => set({ isDirty: false }),
  reset: () => set(initialState),
}))
