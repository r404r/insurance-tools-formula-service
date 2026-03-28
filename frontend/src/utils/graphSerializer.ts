import type { Node, Edge } from '@xyflow/react'
import type { FormulaGraph } from '../types/formula'

export interface ReactFlowData {
  nodes: Node[]
  edges: Edge[]
}

/**
 * Convert backend FormulaGraph to react-flow nodes/edges.
 * Backend stores positions in layout.positions, not on individual nodes.
 */
export function apiToReactFlow(graph: FormulaGraph): ReactFlowData {
  const positions = graph.layout?.positions ?? {}

  const nodes: Node[] = graph.nodes.map((node) => ({
    id: node.id,
    type: 'default',
    position: positions[node.id] ?? { x: 0, y: 0 },
    data: {
      label: node.type,
      nodeType: node.type,
      config: node.config,
    },
  }))

  const edges: Edge[] = graph.edges.map((edge, i) => ({
    id: `edge_${i}`,
    source: edge.source,
    target: edge.target,
    sourceHandle: edge.sourcePort || undefined,
    targetHandle: edge.targetPort || undefined,
  }))

  return { nodes, edges }
}

/**
 * Convert react-flow nodes/edges back to backend FormulaGraph.
 * Positions are stored in layout.positions to match the backend schema.
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
