import { api } from './client'
import type {
  CalculationRequest,
  CalculationResult,
  BatchCalculationRequest,
  BatchCalculationResult,
  FormulaGraph,
  ValidationResult,
} from '../types/formula'

export function calculate(data: CalculationRequest): Promise<CalculationResult> {
  return api.post<CalculationResult>('/calculations', data)
}

export function batchCalculate(data: BatchCalculationRequest): Promise<BatchCalculationResult> {
  return api.post<BatchCalculationResult>('/calculations/batch', data)
}

export function validateFormula(graph: FormulaGraph): Promise<ValidationResult> {
  return api.post<ValidationResult>('/calculations/validate', graph)
}
