import type { Node, Edge } from '@xyflow/react'
import type { FormulaGraph, FormulaNode, FormulaEdge, NodeConfig } from '../types/formula'

export interface ReactFlowData {
  nodes: Node[]
  edges: Edge[]
}

export function apiToReactFlow(graph: FormulaGraph): ReactFlowData {
  const nodes: Node[] = graph.nodes.map((node: FormulaNode) => ({
    id: node.id,
    type: node.type,
    position: { x: node.position.x, y: node.position.y },
    data: {
      label: node.label,
      config: node.config,
      nodeType: node.type,
    },
  }))

  const edges: Edge[] = graph.edges.map((edge: FormulaEdge) => ({
    id: edge.id,
    source: edge.sourceId,
    target: edge.targetId,
    sourceHandle: edge.sourcePort,
    targetHandle: edge.targetPort,
  }))

  return { nodes, edges }
}

export function reactFlowToApi(
  nodes: Node[],
  edges: Edge[],
  outputs: string[]
): FormulaGraph {
  const formulaNodes: FormulaNode[] = nodes.map((node) => ({
    id: node.id,
    type: (node.data.nodeType ?? node.type) as FormulaNode['type'],
    label: (node.data.label as string) ?? '',
    config: (node.data.config as NodeConfig) ?? {},
    position: { x: node.position.x, y: node.position.y },
  }))

  const formulaEdges: FormulaEdge[] = edges.map((edge) => ({
    id: edge.id,
    sourceId: edge.source,
    targetId: edge.target,
    sourcePort: edge.sourceHandle ?? undefined,
    targetPort: edge.targetHandle ?? undefined,
  }))

  return {
    nodes: formulaNodes,
    edges: formulaEdges,
    outputs,
    layout: { autoLayout: false, direction: 'TB' },
  }
}
