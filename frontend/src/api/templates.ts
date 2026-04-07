import { api } from './client'
import type { FormulaTemplate } from '../types/formula'

export function listTemplates(): Promise<FormulaTemplate[]> {
  return api
    .get<{ templates: FormulaTemplate[] }>('/templates')
    .then((r) => r.templates)
}
