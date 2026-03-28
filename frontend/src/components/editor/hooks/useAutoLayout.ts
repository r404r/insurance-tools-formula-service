import dagre from '@dagrejs/dagre'
import type { Edge, Node } from '@xyflow/react'
import { estimateNodeSize as estimateNodeDimensions } from '../nodePresentation'

interface SizedNode {
  id: string
  width: number
  height: number
  position: { x: number; y: number }
}

const LAYER_GAP = 180
const NODE_GAP = 44
const PADDING_X = 48
const PADDING_Y = 40
const SWEEP_PASSES = 4

function estimateNodeSize(node: Node): SizedNode {
  const measuredWidth = node.measured?.width ?? node.width
  const measuredHeight = node.measured?.height ?? node.height
  if (measuredWidth && measuredHeight) {
    return {
      id: node.id,
      width: measuredWidth,
      height: measuredHeight,
      position: node.position,
    }
  }

  const nodeType = String(node.data.nodeType ?? node.type)
  const config = (node.data.config as Record<string, unknown>) ?? {}
  const size = estimateNodeDimensions(nodeType, config)
  return {
    id: node.id,
    width: size.width,
    height: size.height,
    position: node.position,
  }
}

function dagreFallback(nodes: Node[], edges: Edge[]): Node[] {
  const graph = new dagre.graphlib.Graph()
  graph.setDefaultEdgeLabel(() => ({}))
  graph.setGraph({
    rankdir: 'LR',
    align: 'UL',
    nodesep: 56,
    ranksep: 120,
    edgesep: 24,
    marginx: PADDING_X,
    marginy: PADDING_Y,
  })

  nodes.forEach((node) => {
    const size = estimateNodeSize(node)
    graph.setNode(node.id, { width: size.width, height: size.height })
  })
  edges.forEach((edge) => graph.setEdge(edge.source, edge.target))
  dagre.layout(graph)

  return nodes.map((node) => {
    const pos = graph.node(node.id)
    const size = estimateNodeSize(node)
    return {
      ...node,
      position: { x: pos.x - size.width / 2, y: pos.y - size.height / 2 },
    }
  })
}

function buildNeighborMap(edges: Edge[], direction: 'in' | 'out'): Map<string, string[]> {
  const map = new Map<string, string[]>()

  edges.forEach((edge) => {
    const key = direction === 'in' ? edge.target : edge.source
    const value = direction === 'in' ? edge.source : edge.target
    if (!map.has(key)) {
      map.set(key, [])
    }
    map.get(key)?.push(value)
  })

  return map
}

function buildTopologicalOrder(nodes: Node[], edges: Edge[]): string[] | null {
  const indegree = new Map<string, number>()
  const outgoing = buildNeighborMap(edges, 'out')

  nodes.forEach((node) => indegree.set(node.id, 0))
  edges.forEach((edge) => indegree.set(edge.target, (indegree.get(edge.target) ?? 0) + 1))

  const queue = nodes
    .map((node) => node.id)
    .filter((id) => (indegree.get(id) ?? 0) === 0)
    .sort((left, right) => left.localeCompare(right))

  const order: string[] = []

  while (queue.length > 0) {
    const current = queue.shift()
    if (!current) {
      break
    }

    order.push(current)
    for (const target of outgoing.get(current) ?? []) {
      const nextDegree = (indegree.get(target) ?? 0) - 1
      indegree.set(target, nextDegree)
      if (nextDegree === 0) {
        queue.push(target)
        queue.sort((left, right) => left.localeCompare(right))
      }
    }
  }

  return order.length === nodes.length ? order : null
}

function computeLayers(order: string[], edges: Edge[]): Map<string, number> {
  const incoming = buildNeighborMap(edges, 'in')
  const layers = new Map<string, number>()

  order.forEach((nodeId) => {
    const parents = incoming.get(nodeId) ?? []
    let layer = 0
    parents.forEach((parentId) => {
      layer = Math.max(layer, (layers.get(parentId) ?? 0) + 1)
    })
    layers.set(nodeId, layer)
  })

  return layers
}

function buildLayerGroups(order: string[], layers: Map<string, number>, nodeSizes: Map<string, SizedNode>) {
  const groups = new Map<number, string[]>()

  order.forEach((nodeId) => {
    const layer = layers.get(nodeId) ?? 0
    if (!groups.has(layer)) {
      groups.set(layer, [])
    }
    groups.get(layer)?.push(nodeId)
  })

  return [...groups.entries()]
    .sort((left, right) => left[0] - right[0])
    .map(([layer, nodeIds]) => ({
      layer,
      nodeIds: nodeIds.sort((left, right) => {
        const leftY = nodeSizes.get(left)?.position.y ?? 0
        const rightY = nodeSizes.get(right)?.position.y ?? 0
        if (leftY !== rightY) {
          return leftY - rightY
        }
        return left.localeCompare(right)
      }),
    }))
}

function buildIndexMap(layerGroups: { layer: number; nodeIds: string[] }[]) {
  const indexMap = new Map<string, number>()
  layerGroups.forEach(({ nodeIds }) => {
    nodeIds.forEach((nodeId, index) => indexMap.set(nodeId, index))
  })
  return indexMap
}

function barycenter(nodeId: string, neighbors: Map<string, string[]>, indexMap: Map<string, number>): number | null {
  const linked = neighbors.get(nodeId) ?? []
  const values = linked
    .map((neighborId) => indexMap.get(neighborId))
    .filter((value): value is number => value !== undefined)

  if (values.length === 0) {
    return null
  }

  return values.reduce((sum, value) => sum + value, 0) / values.length
}

function reduceCrossings(layerGroups: { layer: number; nodeIds: string[] }[], edges: Edge[], nodeSizes: Map<string, SizedNode>) {
  const incoming = buildNeighborMap(edges, 'in')
  const outgoing = buildNeighborMap(edges, 'out')

  for (let pass = 0; pass < SWEEP_PASSES; pass += 1) {
    let indexMap = buildIndexMap(layerGroups)

    for (let layerIndex = 1; layerIndex < layerGroups.length; layerIndex += 1) {
      layerGroups[layerIndex].nodeIds.sort((left, right) => {
        const leftScore = barycenter(left, incoming, indexMap)
        const rightScore = barycenter(right, incoming, indexMap)
        if (leftScore !== null && rightScore !== null && leftScore !== rightScore) {
          return leftScore - rightScore
        }
        if (leftScore !== null && rightScore === null) {
          return -1
        }
        if (leftScore === null && rightScore !== null) {
          return 1
        }

        const leftY = nodeSizes.get(left)?.position.y ?? 0
        const rightY = nodeSizes.get(right)?.position.y ?? 0
        if (leftY !== rightY) {
          return leftY - rightY
        }
        return left.localeCompare(right)
      })
      indexMap = buildIndexMap(layerGroups)
    }

    for (let layerIndex = layerGroups.length - 2; layerIndex >= 0; layerIndex -= 1) {
      layerGroups[layerIndex].nodeIds.sort((left, right) => {
        const leftScore = barycenter(left, outgoing, indexMap)
        const rightScore = barycenter(right, outgoing, indexMap)
        if (leftScore !== null && rightScore !== null && leftScore !== rightScore) {
          return leftScore - rightScore
        }
        if (leftScore !== null && rightScore === null) {
          return -1
        }
        if (leftScore === null && rightScore !== null) {
          return 1
        }

        const leftY = nodeSizes.get(left)?.position.y ?? 0
        const rightY = nodeSizes.get(right)?.position.y ?? 0
        if (leftY !== rightY) {
          return leftY - rightY
        }
        return left.localeCompare(right)
      })
      indexMap = buildIndexMap(layerGroups)
    }
  }
}

function layerHeight(nodeIds: string[], nodeSizes: Map<string, SizedNode>): number {
  if (nodeIds.length === 0) {
    return 0
  }

  const totalNodeHeights = nodeIds.reduce((sum, nodeId) => sum + (nodeSizes.get(nodeId)?.height ?? 0), 0)
  return totalNodeHeights + NODE_GAP * Math.max(0, nodeIds.length - 1)
}

function positionNodes(
  nodes: Node[],
  layerGroups: { layer: number; nodeIds: string[] }[],
  nodeSizes: Map<string, SizedNode>
): Node[] {
  const layerWidths = layerGroups.map(({ nodeIds }) =>
    nodeIds.reduce((max, nodeId) => Math.max(max, nodeSizes.get(nodeId)?.width ?? 0), 0)
  )
  const layerHeights = layerGroups.map(({ nodeIds }) => layerHeight(nodeIds, nodeSizes))
  const totalHeight = Math.max(...layerHeights, 0)

  const positions = new Map<string, { x: number; y: number }>()
  let currentX = PADDING_X

  layerGroups.forEach(({ nodeIds }, layerIndex) => {
    const width = layerWidths[layerIndex] ?? 0
    const height = layerHeights[layerIndex] ?? 0
    let currentY = PADDING_Y + Math.max(0, (totalHeight - height) / 2)

    nodeIds.forEach((nodeId) => {
      const nodeSize = nodeSizes.get(nodeId)
      if (!nodeSize) {
        return
      }
      positions.set(nodeId, {
        x: currentX,
        y: currentY,
      })
      currentY += nodeSize.height + NODE_GAP
    })

    currentX += width + LAYER_GAP
  })

  return nodes.map((node) => ({
    ...node,
    position: positions.get(node.id) ?? node.position,
  }))
}

export function useAutoLayout() {
  return (nodes: Node[], edges: Edge[]): Node[] => {
    if (nodes.length === 0) {
      return nodes
    }

    const order = buildTopologicalOrder(nodes, edges)
    if (!order) {
      return dagreFallback(nodes, edges)
    }

    const nodeSizes = new Map(nodes.map((node) => [node.id, estimateNodeSize(node)]))
    const layers = computeLayers(order, edges)
    const layerGroups = buildLayerGroups(order, layers, nodeSizes)
    reduceCrossings(layerGroups, edges, nodeSizes)

    return positionNodes(nodes, layerGroups, nodeSizes)
  }
}
