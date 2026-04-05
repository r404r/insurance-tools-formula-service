import dagre from '@dagrejs/dagre'
import type { Edge, Node } from '@xyflow/react'
import { estimateNodeSize as estimateNodeDimensions, getInputPorts } from '../nodePresentation'

interface SizedNode {
  id: string
  width: number
  height: number
  position: { x: number; y: number }
}

// Minimal edge shape used internally; Edge[] satisfies this constraint.
export type MinimalEdge = { source: string; target: string; targetHandle?: string }

const LAYER_GAP = 180
const NODE_GAP = 44
const PADDING_X = 48
const PADDING_Y = 40
const SWEEP_PASSES = 8

// Prefix for dummy nodes inserted to represent intermediate hops of long edges.
// These nodes are invisible and only participate in crossing-reduction sorting.
const DUMMY_PREFIX = '__dummy__'

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

interface NeighborEntry {
  id: string
  portOffset: number // 0.0 (top) to 1.0 (bottom) — the port position on the *current* node
}

/**
 * Build port-offset lookup: for each node, maps port id → fractional Y position (0–1).
 */
function buildPortOffsets(nodes: Node[]): Map<string, Map<string, number>> {
  const result = new Map<string, Map<string, number>>()
  nodes.forEach((node) => {
    const nodeType = String(node.data.nodeType ?? node.type)
    const config = (node.data.config as Record<string, unknown>) ?? {}
    const ports = getInputPorts(nodeType, config)
    const portMap = new Map<string, number>()
    ports.forEach((p) => {
      portMap.set(p.id, parseFloat(p.top) / 100)
    })
    // Output port is always at 50%
    portMap.set('out', 0.5)
    result.set(node.id, portMap)
  })
  return result
}

function buildNeighborMap(
  edges: MinimalEdge[],
  direction: 'in' | 'out',
  portOffsets: Map<string, Map<string, number>>,
): Map<string, NeighborEntry[]> {
  const map = new Map<string, NeighborEntry[]>()

  edges.forEach((edge) => {
    const key = direction === 'in' ? edge.target : edge.source
    const neighborId = direction === 'in' ? edge.source : edge.target
    // Port offset on the current node (key), not the neighbor
    const portId = direction === 'in' ? (edge.targetHandle ?? '') : 'out'
    const offset = portOffsets.get(key)?.get(portId) ?? 0.5

    if (!map.has(key)) {
      map.set(key, [])
    }
    map.get(key)?.push({ id: neighborId, portOffset: offset })
  })

  return map
}

function buildSimpleNeighborMap(edges: MinimalEdge[], direction: 'in' | 'out'): Map<string, string[]> {
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

export function buildTopologicalOrder(nodes: Node[], edges: Edge[]): string[] | null {
  const indegree = new Map<string, number>()
  const outgoing = buildSimpleNeighborMap(edges, 'out')

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

export function computeLayers(order: string[], edges: MinimalEdge[]): Map<string, number> {
  const incoming = buildSimpleNeighborMap(edges, 'in')
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

/**
 * Insert dummy nodes for long edges (edges spanning more than one layer).
 *
 * Standard Sugiyama step: a long edge source→target where target is k>1 layers
 * away is split into a chain source→d1→d2→…→target via k-1 dummy nodes placed
 * at each intermediate layer. The dummies participate in crossing-reduction
 * sorting so that the real nodes in intermediate layers are ordered correctly
 * relative to the invisible paths of long edges.
 *
 * Returns the expanded virtual-edge list (real edges for adjacent-layer pairs
 * plus dummy chains for long edges). Mutates layerGroups and nodeSizes in place.
 */
export function insertDummyNodes(
  layerGroups: { layer: number; nodeIds: string[] }[],
  edges: MinimalEdge[],
  layers: Map<string, number>,
  nodeSizes: Map<string, SizedNode>,
): MinimalEdge[] {
  const layerIndexByNumber = new Map(layerGroups.map((g, i) => [g.layer, i]))
  const virtualEdges: MinimalEdge[] = []

  edges.forEach((edge, i) => {
    if (!layers.has(edge.source) || !layers.has(edge.target)) return
    const srcLayer = layers.get(edge.source)!
    const tgtLayer = layers.get(edge.target)!
    const span = tgtLayer - srcLayer

    if (span <= 1) {
      // Adjacent-layer edge: keep as-is.
      virtualEdges.push(edge)
      return
    }

    // Long edge: insert one dummy node per intermediate layer.
    let prevId = edge.source
    for (let l = srcLayer + 1; l < tgtLayer; l++) {
      const dummyId = `${DUMMY_PREFIX}${i}_${l}`
      const groupIdx = layerIndexByNumber.get(l)
      if (groupIdx !== undefined) {
        layerGroups[groupIdx].nodeIds.push(dummyId)
      }
      // Zero-size so dummies don't consume vertical space.
      nodeSizes.set(dummyId, { id: dummyId, width: 0, height: 0, position: { x: 0, y: 0 } })
      virtualEdges.push({ source: prevId, target: dummyId })
      prevId = dummyId
    }
    virtualEdges.push({ source: prevId, target: edge.target, targetHandle: edge.targetHandle })
  })

  return virtualEdges
}

function buildIndexMap(layerGroups: { layer: number; nodeIds: string[] }[]) {
  const indexMap = new Map<string, number>()
  layerGroups.forEach(({ nodeIds }) => {
    nodeIds.forEach((nodeId, index) => indexMap.set(nodeId, index))
  })
  return indexMap
}

function barycenter(
  nodeId: string,
  neighbors: Map<string, NeighborEntry[]>,
  indexMap: Map<string, number>,
): number | null {
  const linked = neighbors.get(nodeId) ?? []
  const values = linked
    .map((entry) => {
      const idx = indexMap.get(entry.id)
      if (idx === undefined) return undefined
      // Shift by port offset relative to center (0.5) so edges targeting
      // higher ports pull the node downward and vice-versa.
      return idx + (entry.portOffset - 0.5)
    })
    .filter((value): value is number => value !== undefined)

  if (values.length === 0) {
    return null
  }

  return values.reduce((sum, value) => sum + value, 0) / values.length
}

function reduceCrossings(
  layerGroups: { layer: number; nodeIds: string[] }[],
  edges: MinimalEdge[],
  nodeSizes: Map<string, SizedNode>,
  portOffsets: Map<string, Map<string, number>>,
) {
  const incoming = buildNeighborMap(edges, 'in', portOffsets)
  const outgoing = buildNeighborMap(edges, 'out', portOffsets)

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

/**
 * Enforce minimum vertical gaps between nodes so they don't overlap.
 */
export function enforceMinGaps(
  nodeIds: string[],
  yMap: Map<string, number>,
  nodeSizes: Map<string, SizedNode>,
) {
  const realNodes = nodeIds.filter((id) => !id.startsWith(DUMMY_PREFIX))
  if (realNodes.length <= 1) return

  realNodes.sort((a, b) => (yMap.get(a) ?? 0) - (yMap.get(b) ?? 0))

  for (let i = 1; i < realNodes.length; i++) {
    const prevId = realNodes[i - 1]
    const curId = realNodes[i]
    const prevBottom = (yMap.get(prevId) ?? 0) + (nodeSizes.get(prevId)?.height ?? 0) + NODE_GAP
    const curY = yMap.get(curId) ?? 0
    if (curY < prevBottom) {
      yMap.set(curId, prevBottom)
    }
  }
}

function positionNodes(
  nodes: Node[],
  layerGroups: { layer: number; nodeIds: string[] }[],
  nodeSizes: Map<string, SizedNode>,
  edges: MinimalEdge[],
): Node[] {
  const layerWidths = layerGroups.map(({ nodeIds }) =>
    nodeIds
      .filter((id) => !id.startsWith(DUMMY_PREFIX))
      .reduce((max, nodeId) => Math.max(max, nodeSizes.get(nodeId)?.width ?? 0), 0),
  )

  // Build incoming neighbor map for Y-coordinate averaging
  const incoming = buildSimpleNeighborMap(edges, 'in')

  const yPositions = new Map<string, number>()

  // Layer 0: stack nodes top-down as the initial reference
  const layer0 = layerGroups[0]
  if (layer0) {
    let currentY = PADDING_Y
    layer0.nodeIds.forEach((nodeId) => {
      if (nodeId.startsWith(DUMMY_PREFIX)) return
      yPositions.set(nodeId, currentY)
      currentY += (nodeSizes.get(nodeId)?.height ?? 0) + NODE_GAP
    })
  }

  // Layers 1+: position each node at the average Y of its upstream neighbors
  for (let layerIndex = 1; layerIndex < layerGroups.length; layerIndex++) {
    const { nodeIds } = layerGroups[layerIndex]

    // Compute ideal Y for each node (including dummies)
    nodeIds.forEach((nodeId) => {
      const parents = incoming.get(nodeId) ?? []
      const parentYs = parents
        .map((p) => {
          const py = yPositions.get(p)
          if (py === undefined) return undefined
          const ph = nodeSizes.get(p)?.height ?? 0
          // Use parent center Y
          return py + ph / 2
        })
        .filter((v): v is number => v !== undefined)

      if (parentYs.length > 0) {
        const avgCenterY = parentYs.reduce((s, v) => s + v, 0) / parentYs.length
        const selfHeight = nodeSizes.get(nodeId)?.height ?? 0
        yPositions.set(nodeId, avgCenterY - selfHeight / 2)
      } else {
        // No upstream neighbors: place at PADDING_Y (will be adjusted by gap enforcement)
        yPositions.set(nodeId, PADDING_Y)
      }
    })

    // Enforce minimum gaps between real nodes
    enforceMinGaps(nodeIds, yPositions, nodeSizes)

    // Re-sync dummy Y positions to the average of their gap-enforced real neighbors,
    // so downstream layers inherit accurate parent-center coordinates.
    nodeIds.forEach((nodeId) => {
      if (!nodeId.startsWith(DUMMY_PREFIX)) return
      const parents = incoming.get(nodeId) ?? []
      const parentYs = parents
        .map((p) => {
          const py = yPositions.get(p)
          if (py === undefined) return undefined
          return py + (nodeSizes.get(p)?.height ?? 0) / 2
        })
        .filter((v): v is number => v !== undefined)
      if (parentYs.length > 0) {
        yPositions.set(nodeId, parentYs.reduce((s, v) => s + v, 0) / parentYs.length)
      }
    })
  }

  // Ensure no negative Y positions
  let minY = Infinity
  yPositions.forEach((y) => {
    if (y < minY) minY = y
  })
  if (minY < PADDING_Y) {
    const shift = PADDING_Y - minY
    yPositions.forEach((y, id) => yPositions.set(id, y + shift))
  }

  // Assign X positions
  const positions = new Map<string, { x: number; y: number }>()
  let currentX = PADDING_X

  layerGroups.forEach(({ nodeIds }, layerIndex) => {
    const width = layerWidths[layerIndex] ?? 0
    nodeIds.forEach((nodeId) => {
      if (nodeId.startsWith(DUMMY_PREFIX)) return
      const y = yPositions.get(nodeId) ?? PADDING_Y
      positions.set(nodeId, { x: currentX, y })
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

    if (nodes.some((n) => n.id.startsWith(DUMMY_PREFIX))) {
      console.warn('useAutoLayout: node IDs starting with "__dummy__" conflict with internal layout nodes; falling back to dagre')
      return dagreFallback(nodes, edges)
    }

    const order = buildTopologicalOrder(nodes, edges)
    if (!order) {
      return dagreFallback(nodes, edges)
    }

    const nodeSizes = new Map(nodes.map((node) => [node.id, estimateNodeSize(node)]))
    const layers = computeLayers(order, edges)
    const layerGroups = buildLayerGroups(order, layers, nodeSizes)

    // Insert dummy nodes for long edges (standard Sugiyama step).
    // This ensures intermediate layers "see" long edges during crossing reduction.
    const virtualEdges = insertDummyNodes(layerGroups, edges, layers, nodeSizes)

    const portOffsets = buildPortOffsets(nodes)
    reduceCrossings(layerGroups, virtualEdges, nodeSizes, portOffsets)

    return positionNodes(nodes, layerGroups, nodeSizes, virtualEdges)
  }
}
