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
  variableName: string
  dataType: 'number' | 'string' | 'boolean'
  defaultValue?: string
  description?: string
}

export interface ConstantConfig {
  value: string
  dataType: 'number' | 'string' | 'boolean'
}

export interface OperatorConfig {
  operator: OperatorKind
}

export interface FunctionConfig {
  function: FunctionKind
  precision?: number
}

export interface SubFormulaConfig {
  formulaId: string
  versionNumber?: number
}

export interface TableLookupConfig {
  tableId: string
  lookupColumns: string[]
  resultColumn: string
  interpolation: 'none' | 'linear'
}

export interface ConditionalConfig {
  conditions: ConditionalBranch[]
}

export interface ConditionalBranch {
  comparison: 'eq' | 'ne' | 'gt' | 'gte' | 'lt' | 'lte'
  value: string
  resultNodeId: string
}

export interface AggregateConfig {
  aggregation: AggregateKind
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

export interface Position {
  x: number
  y: number
}

export interface FormulaNode {
  id: string
  type: NodeType
  label: string
  config: NodeConfig
  position: Position
}

export interface FormulaEdge {
  id: string
  sourceId: string
  targetId: string
  sourcePort?: string
  targetPort?: string
}

export interface GraphLayout {
  autoLayout: boolean
  direction: 'TB' | 'LR'
}

export interface FormulaGraph {
  nodes: FormulaNode[]
  edges: FormulaEdge[]
  outputs: string[]
  layout: GraphLayout
}

export type VersionState = 'draft' | 'published' | 'archived'

export interface FormulaVersion {
  formulaId: string
  versionNumber: number
  state: VersionState
  graph: FormulaGraph
  changeNote: string
  createdBy: string
  createdAt: string
  publishedAt?: string
}

export interface Formula {
  id: string
  name: string
  domain: InsuranceDomain
  description: string
  currentVersion: number
  createdBy: string
  createdAt: string
  updatedAt: string
}

export type Role = 'admin' | 'editor' | 'viewer'

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
  columns: LookupColumn[]
  rows: Record<string, string>[]
  createdBy: string
  createdAt: string
  updatedAt: string
}

export interface LookupColumn {
  name: string
  dataType: 'number' | 'string'
  isKey: boolean
}

export interface CalculationRequest {
  formulaId: string
  versionNumber?: number
  inputs: Record<string, string>
  precision?: number
}

export interface CalculationResult {
  outputs: Record<string, string>
  executionTimeMs: number
  nodesEvaluated: number
  parallelLevels: number
}

export interface BatchCalculationRequest {
  formulaId: string
  versionNumber?: number
  inputSets: Record<string, string>[]
  precision?: number
}

export interface BatchCalculationResult {
  results: CalculationResult[]
  totalTimeMs: number
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
