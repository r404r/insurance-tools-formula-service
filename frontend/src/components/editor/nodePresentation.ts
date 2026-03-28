import type { NodeType } from '../../types/formula'

export const NODE_COLORS: Record<string, { bg: string; border: string }> = {
  variable: { bg: '#dbeafe', border: '#3b82f6' },
  constant: { bg: '#fef3c7', border: '#f59e0b' },
  operator: { bg: '#fce7f3', border: '#ec4899' },
  function: { bg: '#d1fae5', border: '#10b981' },
  subFormula: { bg: '#e0e7ff', border: '#6366f1' },
  tableLookup: { bg: '#fae8ff', border: '#a855f7' },
  conditional: { bg: '#ffedd5', border: '#f97316' },
  aggregate: { bg: '#ccfbf1', border: '#14b8a6' },
}

const OP_SYMBOLS: Record<string, string> = {
  add: '+',
  subtract: '-',
  multiply: 'x',
  divide: '/',
  power: '^',
  modulo: '%',
}

export interface FormulaNodeData {
  [key: string]: unknown
  label: string
  nodeType: NodeType
  config: Record<string, unknown>
}

export interface PortDef {
  id: string
  top: string
  label: string
}

export function nodeLabel(type: string, config: Record<string, unknown>): string {
  switch (type) {
    case 'variable':
      return `${config.name ?? 'var'}`
    case 'constant':
      return `${config.value ?? '?'}`
    case 'operator':
      return OP_SYMBOLS[config.op as string] ?? (config.op as string) ?? '?'
    case 'function': {
      const fn = (config.fn as string) ?? '?'
      const args = config.args as Record<string, string> | undefined
      const places = args?.places
      return places ? `${fn}(${places})` : fn
    }
    case 'subFormula':
      return `sub:${config.formulaId ?? '?'}`
    case 'tableLookup':
      return `lookup(${config.column ?? '?'})`
    case 'conditional':
      return `if ${config.comparator ?? '?'}`
    case 'aggregate':
      return `sum:${config.fn ?? '?'}`
    default:
      return type
  }
}

export function createNodeData(type: NodeType, config: Record<string, unknown>): FormulaNodeData {
  return {
    label: nodeLabel(type, config),
    nodeType: type,
    config,
  }
}

export function getInputPorts(nodeType: string, config: Record<string, unknown>): PortDef[] {
  switch (nodeType) {
    case 'operator':
      return [
        { id: 'left', top: '32%', label: 'L' },
        { id: 'right', top: '68%', label: 'R' },
      ]
    case 'function':
      if (config.fn === 'min' || config.fn === 'max') {
        return [
          { id: 'left', top: '32%', label: 'L' },
          { id: 'right', top: '68%', label: 'R' },
        ]
      }
      return [{ id: 'in', top: '50%', label: 'In' }]
    case 'subFormula':
      return [{ id: 'in', top: '50%', label: 'In' }]
    case 'tableLookup':
      return [{ id: 'key', top: '50%', label: 'Key' }]
    case 'conditional':
      return [
        { id: 'condition', top: '18%', label: 'If' },
        { id: 'conditionRight', top: '40%', label: 'Cmp' },
        { id: 'thenValue', top: '62%', label: 'Then' },
        { id: 'elseValue', top: '84%', label: 'Else' },
      ]
    case 'aggregate':
      return [{ id: 'items', top: '50%', label: 'Items' }]
    default:
      return []
  }
}

export function defaultNodeConfig(type: NodeType): Record<string, unknown> {
  switch (type) {
    case 'variable':
      return { name: '', dataType: 'decimal' }
    case 'constant':
      return { value: '0' }
    case 'operator':
      return { op: 'add' }
    case 'function':
      return { fn: 'round', args: { places: '18' } }
    case 'subFormula':
      return { formulaId: '' }
    case 'tableLookup':
      return { tableId: '', lookupKey: '', column: '' }
    case 'conditional':
      return { comparator: 'gt' }
    case 'aggregate':
      return { fn: 'sum', range: '' }
    default:
      return {}
  }
}
