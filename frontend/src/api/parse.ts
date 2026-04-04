import { api } from './client'
import type { FormulaGraph } from '../types/formula'

export interface ParseResponse {
  graph: FormulaGraph
}

export function parseFormula(text: string): Promise<ParseResponse> {
  return api.post<ParseResponse>('/parse', { text })
}
