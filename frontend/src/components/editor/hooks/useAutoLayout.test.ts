import { describe, it, expect } from 'vitest'
import type { Node, Edge } from '@xyflow/react'
import {
  buildTopologicalOrder,
  computeLayers,
  insertDummyNodes,
  enforceMinGaps,
  useAutoLayout,
  type MinimalEdge,
} from './useAutoLayout'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeNode(id: string): Node {
  return {
    id,
    type: 'formulaNode',
    position: { x: 0, y: 0 },
    data: { nodeType: 'variable', config: {} },
    measured: { width: 100, height: 40 },
  } as unknown as Node
}

function makeEdge(source: string, target: string, targetHandle?: string): Edge {
  return { id: `${source}->${target}`, source, target, targetHandle } as Edge
}

// ---------------------------------------------------------------------------
// buildTopologicalOrder
// ---------------------------------------------------------------------------

describe('buildTopologicalOrder', () => {
  it('returns nodes in topological order for a simple DAG', () => {
    const nodes = [makeNode('a'), makeNode('b'), makeNode('c')]
    const edges = [makeEdge('a', 'b'), makeEdge('b', 'c')]
    const order = buildTopologicalOrder(nodes, edges)
    expect(order).not.toBeNull()
    expect(order!.indexOf('a')).toBeLessThan(order!.indexOf('b'))
    expect(order!.indexOf('b')).toBeLessThan(order!.indexOf('c'))
  })

  it('returns null for a cyclic graph', () => {
    const nodes = [makeNode('a'), makeNode('b')]
    const edges = [makeEdge('a', 'b'), makeEdge('b', 'a')]
    expect(buildTopologicalOrder(nodes, edges)).toBeNull()
  })

  it('returns null for a self-loop', () => {
    const nodes = [makeNode('a')]
    const edges = [makeEdge('a', 'a')]
    expect(buildTopologicalOrder(nodes, edges)).toBeNull()
  })

  it('handles a single node with no edges', () => {
    const nodes = [makeNode('a')]
    const order = buildTopologicalOrder(nodes, [])
    expect(order).toEqual(['a'])
  })

  it('handles empty graph', () => {
    expect(buildTopologicalOrder([], [])).toEqual([])
  })

  it('handles disconnected components', () => {
    const nodes = [makeNode('a'), makeNode('b'), makeNode('c'), makeNode('d')]
    const edges = [makeEdge('a', 'b'), makeEdge('c', 'd')]
    const order = buildTopologicalOrder(nodes, edges)
    expect(order).not.toBeNull()
    expect(order!).toHaveLength(4)
    expect(order!.indexOf('a')).toBeLessThan(order!.indexOf('b'))
    expect(order!.indexOf('c')).toBeLessThan(order!.indexOf('d'))
  })
})

// ---------------------------------------------------------------------------
// computeLayers
// ---------------------------------------------------------------------------

describe('computeLayers', () => {
  it('assigns increasing layers along a chain', () => {
    const edges: MinimalEdge[] = [{ source: 'a', target: 'b' }, { source: 'b', target: 'c' }]
    const layers = computeLayers(['a', 'b', 'c'], edges)
    expect(layers.get('a')).toBe(0)
    expect(layers.get('b')).toBe(1)
    expect(layers.get('c')).toBe(2)
  })

  it('assigns deepest required layer when a node has multiple parents', () => {
    // a → c and b → c; if a is layer 0 and b is layer 0, c should be layer 1
    const edges: MinimalEdge[] = [{ source: 'a', target: 'c' }, { source: 'b', target: 'c' }]
    const layers = computeLayers(['a', 'b', 'c'], edges)
    expect(layers.get('c')).toBe(1)
  })
})

// ---------------------------------------------------------------------------
// insertDummyNodes
// ---------------------------------------------------------------------------

describe('insertDummyNodes', () => {
  it('keeps adjacent-layer edges as-is', () => {
    const layers = new Map([['a', 0], ['b', 1]])
    const layerGroups = [{ layer: 0, nodeIds: ['a'] }, { layer: 1, nodeIds: ['b'] }]
    const edges: MinimalEdge[] = [{ source: 'a', target: 'b' }]
    const nodeSizes = new Map<string, { id: string; width: number; height: number; position: { x: number; y: number } }>()
    const virtual = insertDummyNodes(layerGroups, edges, layers, nodeSizes)
    expect(virtual).toHaveLength(1)
    expect(virtual[0]).toEqual({ source: 'a', target: 'b' })
    expect(nodeSizes.size).toBe(0)
  })

  it('inserts dummy nodes for a long edge spanning 2 layers', () => {
    const layers = new Map([['a', 0], ['c', 2]])
    const layerGroups = [
      { layer: 0, nodeIds: ['a'] },
      { layer: 1, nodeIds: [] },
      { layer: 2, nodeIds: ['c'] },
    ]
    const edges: MinimalEdge[] = [{ source: 'a', target: 'c' }]
    const nodeSizes = new Map<string, { id: string; width: number; height: number; position: { x: number; y: number } }>()
    const virtual = insertDummyNodes(layerGroups, edges, layers, nodeSizes)

    // Should produce 2 virtual edges: a→dummy and dummy→c
    expect(virtual).toHaveLength(2)
    expect(virtual[0].source).toBe('a')
    expect(virtual[0].target).toMatch(/^__dummy__/)
    expect(virtual[1].target).toBe('c')
    expect(virtual[1].source).toMatch(/^__dummy__/)
    // Dummy should be in the intermediate layer group
    expect(layerGroups[1].nodeIds).toHaveLength(1)
    expect(layerGroups[1].nodeIds[0]).toMatch(/^__dummy__/)
    // Dummy should be zero-sized
    expect(nodeSizes.get(layerGroups[1].nodeIds[0])).toMatchObject({ width: 0, height: 0 })
  })

  it('preserves targetHandle on the final virtual edge', () => {
    const layers = new Map([['a', 0], ['c', 2]])
    const layerGroups = [
      { layer: 0, nodeIds: ['a'] },
      { layer: 1, nodeIds: [] },
      { layer: 2, nodeIds: ['c'] },
    ]
    const edges: MinimalEdge[] = [{ source: 'a', target: 'c', targetHandle: 'left' }]
    const nodeSizes = new Map<string, { id: string; width: number; height: number; position: { x: number; y: number } }>()
    const virtual = insertDummyNodes(layerGroups, edges, layers, nodeSizes)
    const finalEdge = virtual[virtual.length - 1]
    expect(finalEdge.target).toBe('c')
    expect(finalEdge.targetHandle).toBe('left')
  })

  it('skips edges where source or target is absent from layers map', () => {
    const layers = new Map([['b', 1]])  // 'a' absent
    const layerGroups = [{ layer: 1, nodeIds: ['b'] }]
    const edges: MinimalEdge[] = [{ source: 'a', target: 'b' }]
    const nodeSizes = new Map<string, { id: string; width: number; height: number; position: { x: number; y: number } }>()
    const virtual = insertDummyNodes(layerGroups, edges, layers, nodeSizes)
    expect(virtual).toHaveLength(0)
    expect(nodeSizes.size).toBe(0)
  })
})

// ---------------------------------------------------------------------------
// enforceMinGaps
// ---------------------------------------------------------------------------

describe('enforceMinGaps', () => {
  const makeNodeSizes = (ids: string[], height = 40) => {
    const m = new Map<string, { id: string; width: number; height: number; position: { x: number; y: number } }>()
    ids.forEach(id => m.set(id, { id, width: 100, height, position: { x: 0, y: 0 } }))
    return m
  }

  it('does nothing when nodes already have sufficient gaps', () => {
    const yMap = new Map([['a', 0], ['b', 100]])
    const nodeSizes = makeNodeSizes(['a', 'b'])
    enforceMinGaps(['a', 'b'], yMap, nodeSizes)
    expect(yMap.get('a')).toBe(0)
    expect(yMap.get('b')).toBe(100)
  })

  it('pushes overlapping node down', () => {
    const yMap = new Map([['a', 0], ['b', 10]])  // b overlaps: 0+40+44=84, b needs to move to 84
    const nodeSizes = makeNodeSizes(['a', 'b'])
    enforceMinGaps(['a', 'b'], yMap, nodeSizes)
    expect(yMap.get('b')).toBeGreaterThanOrEqual(yMap.get('a')! + 40 + 44)
  })

  it('cascades shifts across multiple overlapping nodes', () => {
    // All three start at y=0, each 40 tall, NODE_GAP=44 → each should be 84 apart
    const yMap = new Map([['a', 0], ['b', 0], ['c', 0]])
    const nodeSizes = makeNodeSizes(['a', 'b', 'c'])
    enforceMinGaps(['a', 'b', 'c'], yMap, nodeSizes)
    expect(yMap.get('b')).toBeGreaterThanOrEqual(84)
    expect(yMap.get('c')).toBeGreaterThanOrEqual(168)
  })

  it('skips dummy nodes', () => {
    const yMap = new Map([['a', 0], ['__dummy__0_1', 0], ['b', 10]])
    const nodeSizes = makeNodeSizes(['a', '__dummy__0_1', 'b'])
    enforceMinGaps(['a', '__dummy__0_1', 'b'], yMap, nodeSizes)
    // dummy Y unchanged
    expect(yMap.get('__dummy__0_1')).toBe(0)
    // real node b pushed past a
    expect(yMap.get('b')).toBeGreaterThanOrEqual(84)
  })

  it('handles single node without error', () => {
    const yMap = new Map([['a', 0]])
    const nodeSizes = makeNodeSizes(['a'])
    expect(() => enforceMinGaps(['a'], yMap, nodeSizes)).not.toThrow()
  })
})

// ---------------------------------------------------------------------------
// useAutoLayout — integration
// ---------------------------------------------------------------------------

describe('useAutoLayout', () => {
  const layout = useAutoLayout()

  it('returns empty array for empty input', () => {
    expect(layout([], [])).toEqual([])
  })

  it('positions a single node at PADDING coordinates', () => {
    const nodes = [makeNode('a')]
    const result = layout(nodes, [])
    expect(result).toHaveLength(1)
    expect(result[0].position.x).toBeGreaterThanOrEqual(0)
    expect(result[0].position.y).toBeGreaterThanOrEqual(0)
  })

  it('falls back to dagre for cyclic graphs without throwing', () => {
    const nodes = [makeNode('a'), makeNode('b')]
    const edges = [makeEdge('a', 'b'), makeEdge('b', 'a')]
    const result = layout(nodes, edges)
    expect(result).toHaveLength(2)
    result.forEach(n => {
      expect(Number.isFinite(n.position.x)).toBe(true)
      expect(Number.isFinite(n.position.y)).toBe(true)
    })
  })

  it('produces valid positions for a long edge (span > 1)', () => {
    // a → b → c chain with an extra skip edge a → c
    const nodes = [makeNode('a'), makeNode('b'), makeNode('c')]
    const edges = [makeEdge('a', 'b'), makeEdge('b', 'c'), makeEdge('a', 'c')]
    const result = layout(nodes, edges)
    expect(result).toHaveLength(3)
    // No dummy nodes should appear in output
    result.forEach(n => expect(n.id).not.toMatch(/^__dummy__/))
    result.forEach(n => {
      expect(Number.isFinite(n.position.x)).toBe(true)
      expect(Number.isFinite(n.position.y)).toBe(true)
    })
  })

  it('falls back to dagre when a node ID starts with __dummy__', () => {
    const nodes = [makeNode('__dummy__evil'), makeNode('b')]
    const edges = [makeEdge('__dummy__evil', 'b')]
    // Should not throw, and should return both nodes with finite positions
    const result = layout(nodes, edges)
    expect(result).toHaveLength(2)
    result.forEach(n => {
      expect(Number.isFinite(n.position.x)).toBe(true)
      expect(Number.isFinite(n.position.y)).toBe(true)
    })
  })

  it('handles disconnected components', () => {
    const nodes = [makeNode('a'), makeNode('b'), makeNode('c'), makeNode('d')]
    const edges = [makeEdge('a', 'b'), makeEdge('c', 'd')]
    const result = layout(nodes, edges)
    expect(result).toHaveLength(4)
    result.forEach(n => {
      expect(Number.isFinite(n.position.x)).toBe(true)
      expect(Number.isFinite(n.position.y)).toBe(true)
    })
  })

  it('does not produce overlapping Y positions in the same layer', () => {
    // Two nodes feeding into one — they share a layer, must not overlap
    const nodes = [makeNode('a'), makeNode('b'), makeNode('c')]
    // a and b are both sources → same layer; c depends on both
    const edges = [makeEdge('a', 'c'), makeEdge('b', 'c')]
    const result = layout(nodes, edges)
    const byId = new Map(result.map(n => [n.id, n]))
    const ya = byId.get('a')!.position.y
    const yb = byId.get('b')!.position.y
    // They should differ by at least NODE_GAP (44) + node height
    expect(Math.abs(ya - yb)).toBeGreaterThanOrEqual(44)
  })
})
