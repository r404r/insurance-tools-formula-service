import dagre from '@dagrejs/dagre'
import type { Node, Edge } from '@xyflow/react'

export function useAutoLayout() {
  return (nodes: Node[], edges: Edge[]): Node[] => {
    const g = new dagre.graphlib.Graph()
    g.setDefaultEdgeLabel(() => ({}))
    g.setGraph({ rankdir: 'TB', nodesep: 80, ranksep: 100 })
    nodes.forEach((n) => g.setNode(n.id, { width: 180, height: 60 }))
    edges.forEach((e) => g.setEdge(e.source, e.target))
    dagre.layout(g)
    return nodes.map((n) => {
      const pos = g.node(n.id)
      return { ...n, position: { x: pos.x - 90, y: pos.y - 30 } }
    })
  }
}
