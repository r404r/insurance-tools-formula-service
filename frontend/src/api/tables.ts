import { api } from './client'
import type { LookupTable } from '../types/formula'

export interface ListTablesResponse {
  tables: LookupTable[]
}

export interface CreateTableData {
  name: string
  domain: string
  tableType: string
  data: unknown
}

export interface UpdateTableData {
  name: string
  tableType: string
  data: unknown
}

export function listTables(domain?: string): Promise<ListTablesResponse> {
  const qs = domain ? `?domain=${encodeURIComponent(domain)}` : ''
  return api.get<ListTablesResponse>(`/tables${qs}`)
}

export function getTable(id: string): Promise<LookupTable> {
  return api.get<LookupTable>(`/tables/${id}`)
}

export function createTable(data: CreateTableData): Promise<LookupTable> {
  return api.post<LookupTable>('/tables', data)
}

export function updateTable(id: string, data: UpdateTableData): Promise<LookupTable> {
  return api.put<LookupTable>(`/tables/${id}`, data)
}

export function deleteTable(id: string): Promise<void> {
  return api.delete<void>(`/tables/${id}`)
}
