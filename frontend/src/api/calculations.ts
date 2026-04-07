import { api } from './client'
import type {
  CalculationRequest,
  CalculationResult,
  BatchCalculationRequest,
  BatchCalculationResult,
  BatchTestRequest,
  BatchTestResponse,
  FormulaGraph,
  ValidationResult,
} from '../types/formula'

export function calculate(data: CalculationRequest): Promise<CalculationResult> {
  return api.post<CalculationResult>('/calculate', data)
}

export function batchCalculate(data: BatchCalculationRequest): Promise<BatchCalculationResult> {
  return api.post<BatchCalculationResult>('/calculate/batch', data)
}

export function batchTest(data: BatchTestRequest): Promise<BatchTestResponse> {
  return api.post<BatchTestResponse>('/calculate/batch-test', data)
}

export function validateFormula(graph: FormulaGraph): Promise<ValidationResult> {
  return api.post<ValidationResult>('/calculate/validate', graph)
}
