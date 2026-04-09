import { createContext, useContext } from 'react'

export interface ValidationState {
  invalidNodeIds: ReadonlySet<string>
  warnNodeIds: ReadonlySet<string>
}

const emptyState: ValidationState = {
  invalidNodeIds: new Set(),
  warnNodeIds: new Set(),
}

export const ValidationContext = createContext<ValidationState>(emptyState)

export function useValidation(): ValidationState {
  return useContext(ValidationContext)
}
