import { api } from './client'
import type { Formula, InsuranceDomain } from '../types/formula'

export interface ListFormulasParams {
  domain?: InsuranceDomain
  search?: string
  page?: number
  pageSize?: number
}

export interface ListFormulasResponse {
  formulas: Formula[]
  total: number
}

export interface ListAllFormulaIDsParams {
  domain?: InsuranceDomain
  search?: string
}

export interface CreateFormulaData {
  name: string
  domain: InsuranceDomain
  description: string
}

export interface UpdateFormulaData {
  name?: string
  domain?: InsuranceDomain
  description?: string
}

export function listFormulas(params?: ListFormulasParams): Promise<ListFormulasResponse> {
  const query = new URLSearchParams()
  if (params?.domain) query.set('domain', params.domain)
  if (params?.search) query.set('search', params.search)
  if (params?.pageSize !== undefined) query.set('limit', String(params.pageSize))
  if (params?.page !== undefined && params?.pageSize !== undefined) {
    query.set('offset', String(Math.max(0, params.page - 1) * params.pageSize))
  }
  const qs = query.toString()
  return api.get<ListFormulasResponse>(`/formulas${qs ? `?${qs}` : ''}`)
}

// Exporting needs IDs for every matching formula. Keep fetching until the
// server's declared total is reached (or a short page protects us from a
// concurrent deletion) rather than silently truncating at one page.
export async function listAllFormulaIds(params?: ListAllFormulaIDsParams): Promise<string[]> {
  const pageSize = 500
  const ids: string[] = []
  let offset = 0
  let total = Infinity

  while (offset < total) {
    const query = new URLSearchParams()
    if (params?.domain) query.set('domain', params.domain)
    if (params?.search) query.set('search', params.search)
    query.set('limit', String(pageSize))
    query.set('offset', String(offset))
    const response = await api.get<ListFormulasResponse>(`/formulas?${query.toString()}`)
    const formulas = response.formulas ?? []
    ids.push(...formulas.map((formula) => formula.id))
    total = response.total
    if (formulas.length < pageSize) break
    offset += formulas.length
  }

  return ids
}

export function createFormula(data: CreateFormulaData): Promise<Formula> {
  return api.post<Formula>('/formulas', data)
}

export function getFormula(id: string): Promise<Formula> {
  return api.get<Formula>(`/formulas/${id}`)
}

export function updateFormula(id: string, data: UpdateFormulaData): Promise<Formula> {
  return api.put<Formula>(`/formulas/${id}`, data)
}

export function deleteFormula(id: string): Promise<void> {
  return api.delete<void>(`/formulas/${id}`)
}
