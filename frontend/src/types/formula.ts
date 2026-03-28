export type InsuranceDomain = 'life' | 'property' | 'auto'

export type NodeType =
  | 'variable'
  | 'constant'
  | 'operator'
  | 'function'
  | 'subFormula'
  | 'tableLookup'
  | 'conditional'
  | 'aggregate'

export type OperatorKind = 'add' | 'subtract' | 'multiply' | 'divide' | 'modulo' | 'power'

export type FunctionKind =
  | 'round'
  | 'ceil'
  | 'floor'
  | 'abs'
  | 'min'
  | 'max'
  | 'log'
  | 'exp'
  | 'sqrt'

export type AggregateKind = 'sum' | 'avg' | 'min' | 'max' | 'count'

export interface VariableConfig {
  name: string
  dataType: 'integer' | 'decimal' | 'string' | 'boolean'
}

export interface ConstantConfig {
  value: string
}

export interface OperatorConfig {
  op: OperatorKind
}

export interface FunctionConfig {
  fn: FunctionKind
  args?: Record<string, string>
}

export interface SubFormulaConfig {
  formulaId: string
  version?: number
}

export interface TableLookupConfig {
  tableId: string
  lookupKey: string
  column: string
}

export interface ConditionalConfig {
  comparator: 'eq' | 'ne' | 'gt' | 'ge' | 'lt' | 'le'
}

export interface AggregateConfig {
  fn: 'sum' | 'product' | 'count' | 'avg'
  range: string
}

export type NodeConfig =
  | VariableConfig
  | ConstantConfig
  | OperatorConfig
  | FunctionConfig
  | SubFormulaConfig
  | TableLookupConfig
  | ConditionalConfig
  | AggregateConfig
  | Record<string, unknown>

export interface Position {
  x: number
  y: number
}

// Matches backend domain.FormulaNode (config is json.RawMessage, no label/position on node)
export interface FormulaNode {
  id: string
  type: NodeType
  config: Record<string, unknown>
}

// Matches backend domain.FormulaEdge (source/target, not sourceId/targetId)
export interface FormulaEdge {
  source: string
  target: string
  sourcePort: string
  targetPort: string
}

// Matches backend domain.GraphLayout (positions stored here, not on nodes)
export interface GraphLayout {
  positions: Record<string, Position>
}

export interface FormulaGraph {
  nodes: FormulaNode[]
  edges: FormulaEdge[]
  outputs: string[]
  layout?: GraphLayout
}

export type VersionState = 'draft' | 'published' | 'archived'

// Matches backend domain.FormulaVersion (field is "version", not "versionNumber")
export interface FormulaVersion {
  id: string
  formulaId: string
  version: number
  state: VersionState
  graph: FormulaGraph
  changeNote: string
  createdBy: string
  createdAt: string
}

export interface Formula {
  id: string
  name: string
  domain: InsuranceDomain
  description: string
  createdBy: string
  createdAt: string
  updatedAt: string
}

export type Role = 'admin' | 'editor' | 'reviewer' | 'viewer'

export interface User {
  id: string
  username: string
  role: Role
  createdAt: string
}

export interface LookupTable {
  id: string
  name: string
  domain: InsuranceDomain
  tableType: string
  data: unknown
  createdAt: string
}

export interface CalculationRequest {
  formulaId: string
  version?: number
  inputs: Record<string, string>
  precision?: number
}

export interface CalculationResult {
  result: Record<string, string>
  intermediates: Record<string, string>
  executionTimeMs: number
  nodesEvaluated: number
  parallelLevels: number
}

export interface BatchCalculationRequest {
  formulaId: string
  version?: number
  inputSets: Record<string, string>[]
}

export interface BatchCalculationResult {
  results: CalculationResult[]
}

export interface ValidationResult {
  valid: boolean
  errors: ValidationError[]
}

export interface ValidationError {
  nodeId?: string
  edgeId?: string
  message: string
}

export interface LoginRequest {
  username: string
  password: string
}

export interface RegisterRequest {
  username: string
  password: string
}

export interface AuthResponse {
  token: string
  user: User
}
