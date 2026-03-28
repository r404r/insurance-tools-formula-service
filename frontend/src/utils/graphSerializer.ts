import { MarkerType, type Node, type Edge } from '@xyflow/react'
import type { FormulaGraph } from '../types/formula'

export interface ReactFlowData {
  nodes: Node[]
  edges: Edge[]
}

const OP_SYMBOLS: Record<string, string> = {
  add: '+', subtract: '−', multiply: '×', divide: '÷', power: '^', modulo: '%',
}

const NODE_COLORS: Record<string, { bg: string; border: string }> = {
  variable:    { bg: '#dbeafe', border: '#3b82f6' },  // blue
  constant:    { bg: '#fef3c7', border: '#f59e0b' },  // amber
  operator:    { bg: '#fce7f3', border: '#ec4899' },  // pink
  function:    { bg: '#d1fae5', border: '#10b981' },  // green
  subFormula:  { bg: '#e0e7ff', border: '#6366f1' },  // indigo
  tableLookup: { bg: '#fae8ff', border: '#a855f7' },  // purple
  conditional: { bg: '#ffedd5', border: '#f97316' },  // orange
  aggregate:   { bg: '#ccfbf1', border: '#14b8a6' },  // teal
}

/** Generate a human-readable label from node type + config */
function nodeLabel(type: string, config: Record<string, unknown>): string {
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
      return `Σ ${config.fn ?? '?'}`
    default:
      return type
  }
}

/**
 * Convert backend FormulaGraph to react-flow nodes/edges.
 */
export function apiToReactFlow(graph: FormulaGraph): ReactFlowData {
  const positions = graph.layout?.positions ?? {}

  const nodes: Node[] = graph.nodes.map((node) => {
    const colors = NODE_COLORS[node.type] ?? { bg: '#f3f4f6', border: '#9ca3af' }
    return {
      id: node.id,
      type: 'default',
      position: positions[node.id] ?? { x: 0, y: 0 },
      data: {
        label: nodeLabel(node.type, node.config),
        nodeType: node.type,
        config: node.config,
      },
      style: {
        background: colors.bg,
        border: `2px solid ${colors.border}`,
        borderRadius: 8,
        fontSize: 13,
        fontWeight: 600,
        padding: '4px 8px',
        minWidth: 60,
        textAlign: 'center' as const,
      },
    }
  })

  // Don't set sourceHandle/targetHandle — default nodes only have unnamed handles.
  const edges: Edge[] = graph.edges.map((edge, i) => ({
    id: `edge_${i}`,
    source: edge.source,
    target: edge.target,
    animated: false,
    style: { stroke: '#64748b', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#64748b' },
  }))

  return { nodes, edges }
}

/**
 * Convert react-flow nodes/edges back to backend FormulaGraph.
 */
export function reactFlowToApi(
  nodes: Node[],
  edges: Edge[],
  outputs: string[]
): FormulaGraph {
  const positions: Record<string, { x: number; y: number }> = {}

  const formulaNodes = nodes.map((node) => {
    positions[node.id] = { x: node.position.x, y: node.position.y }
    return {
      id: node.id,
      type: ((node.data.nodeType ?? node.type) as string),
      config: (node.data.config as Record<string, unknown>) ?? {},
    }
  })

  const formulaEdges = edges.map((edge) => ({
    source: edge.source,
    target: edge.target,
    sourcePort: edge.sourceHandle ?? '',
    targetPort: edge.targetHandle ?? '',
  }))

  return {
    nodes: formulaNodes as FormulaGraph['nodes'],
    edges: formulaEdges,
    outputs,
    layout: { positions },
  }
}
