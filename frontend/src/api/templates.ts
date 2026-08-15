import { api } from './client'
import type { Formula, FormulaTemplate, InsuranceDomain } from '../types/formula'

export function listTemplates(): Promise<FormulaTemplate[]> {
  return api
    .get<{ templates: FormulaTemplate[] }>('/templates')
    .then((r) => r.templates)
}

export function instantiateTemplate(
  templateId: string,
  data: { name: string; domain: InsuranceDomain; description: string },
): Promise<Formula> {
  return api.post<Formula>(`/templates/${templateId}/instantiate`, data)
}
