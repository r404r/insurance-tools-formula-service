import { MarkerType, type Node, type Edge } from '@xyflow/react'
import type { FormulaGraph } from '../types/formula'
import { createNodeData, getInputPorts } from '../components/editor/nodePresentation'

export interface ReactFlowData {
  nodes: Node[]
  edges: Edge[]
}

function inferEdgeTargetPorts(graph: FormulaGraph): string[] {
  const nodeMap = new Map(
    graph.nodes.map((node) => [node.id, node] as const)
  )
  const assignedPorts = new Map<string, string[]>()

  return graph.edges.map((edge) => {
    if (edge.targetPort) {
      const current = assignedPorts.get(edge.target) ?? []
      current.push(edge.targetPort)
      assignedPorts.set(edge.target, current)
      return edge.targetPort
    }

    const targetNode = nodeMap.get(edge.target)
    if (!targetNode) {
      return 'in'
    }

    const candidatePorts = getInputPorts(targetNode.type, targetNode.config).map((port) => port.id)
    if (candidatePorts.length === 0) {
      return 'in'
    }

    const usedPorts = assignedPorts.get(edge.target) ?? []
    const inferredPort = candidatePorts.find((port) => !usedPorts.includes(port)) ?? candidatePorts[candidatePorts.length - 1]
    assignedPorts.set(edge.target, [...usedPorts, inferredPort])
    return inferredPort
  })
}

/**
 * Convert backend FormulaGraph to react-flow nodes/edges.
 */
export function apiToReactFlow(graph: FormulaGraph): ReactFlowData {
  const positions = graph.layout?.positions ?? {}
  const targetPorts = inferEdgeTargetPorts(graph)

  const nodes: Node[] = graph.nodes.map((node) => {
    return {
      id: node.id,
      type: 'formulaNode',
      position: positions[node.id] ?? { x: 0, y: 0 },
      data: createNodeData(node.type, node.config, node.description),
    }
  })

  const edges: Edge[] = graph.edges.map((edge, i) => ({
    id: `edge_${i}`,
    source: edge.source,
    target: edge.target,
    sourceHandle: edge.sourcePort || 'out',
    targetHandle: targetPorts[i] || 'in',
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
    const desc = String(node.data.description ?? '')
    return {
      id: node.id,
      type: ((node.data.nodeType ?? node.type) as string),
      config: (node.data.config as Record<string, unknown>) ?? {},
      ...(desc ? { description: desc } : {}),
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
