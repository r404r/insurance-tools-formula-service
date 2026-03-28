import dagre from '@dagrejs/dagre'
import type { Node, Edge } from '@xyflow/react'
import { estimateNodeSize as estimateNodeDimensions } from '../nodePresentation'

function estimateNodeSize(node: Node) {
  const measuredWidth = node.measured?.width ?? node.width
  const measuredHeight = node.measured?.height ?? node.height
  if (measuredWidth && measuredHeight) {
    return { width: measuredWidth, height: measuredHeight }
  }

  const nodeType = String(node.data.nodeType ?? node.type)
  const config = (node.data.config as Record<string, unknown>) ?? {}
  return estimateNodeDimensions(nodeType, config)
}

export function useAutoLayout() {
  return (nodes: Node[], edges: Edge[]): Node[] => {
    const g = new dagre.graphlib.Graph()
    g.setDefaultEdgeLabel(() => ({}))
    g.setGraph({
      rankdir: 'LR',
      align: 'UL',
      nodesep: 48,
      ranksep: 100,
      edgesep: 24,
      marginx: 32,
      marginy: 32,
    })
    nodes.forEach((n) => {
      const { width, height } = estimateNodeSize(n)
      g.setNode(n.id, { width, height })
    })
    edges.forEach((e) => g.setEdge(e.source, e.target))
    dagre.layout(g)
    return nodes.map((n) => {
      const pos = g.node(n.id)
      const { width, height } = estimateNodeSize(n)
      return {
        ...n,
        position: { x: pos.x - width / 2, y: pos.y - height / 2 },
      }
    })
  }
}
