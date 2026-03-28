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
  if (params?.page !== undefined) query.set('page', String(params.page))
  if (params?.pageSize !== undefined) query.set('pageSize', String(params.pageSize))
  const qs = query.toString()
  return api.get<ListFormulasResponse>(`/formulas${qs ? `?${qs}` : ''}`)
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
