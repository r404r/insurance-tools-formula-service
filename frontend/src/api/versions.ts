import { api } from './client'
import type { FormulaVersion, FormulaGraph, VersionState } from '../types/formula'

export interface CreateVersionData {
  graph: FormulaGraph
  changeNote: string
}

export interface VersionDiff {
  fromVersion: number
  toVersion: number
  addedNodes: string[]
  removedNodes: string[]
  modifiedNodes: string[]
  addedEdges: string[]
  removedEdges: string[]
}

export function listVersions(formulaId: string): Promise<FormulaVersion[]> {
  return api.get<FormulaVersion[]>(`/formulas/${formulaId}/versions`)
}

export function createVersion(formulaId: string, data: CreateVersionData): Promise<FormulaVersion> {
  return api.post<FormulaVersion>(`/formulas/${formulaId}/versions`, data)
}

export function getVersion(formulaId: string, versionNumber: number): Promise<FormulaVersion> {
  return api.get<FormulaVersion>(`/formulas/${formulaId}/versions/${versionNumber}`)
}

export function updateVersionState(
  formulaId: string,
  versionNumber: number,
  state: VersionState
): Promise<FormulaVersion> {
  return api.patch<FormulaVersion>(`/formulas/${formulaId}/versions/${versionNumber}`, { state })
}

export function getVersionDiff(
  formulaId: string,
  fromVersion: number,
  toVersion: number
): Promise<VersionDiff> {
  return api.get<VersionDiff>(
    `/formulas/${formulaId}/versions/diff?from=${fromVersion}&to=${toVersion}`
  )
}
