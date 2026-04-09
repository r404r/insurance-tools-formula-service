import { describe, it, expect } from 'vitest'
import type { Node, Edge } from '@xyflow/react'
import { detectCycles, findOrphanedNodes, validateGraph } from './graphValidation'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeNode(id: string, nodeType = 'variable'): Node {
  return {
    id,
    type: 'formulaNode',
    position: { x: 0, y: 0 },
    data: { nodeType, config: {} },
  } as unknown as Node
}

function makeEdge(source: string, target: string, sourceHandle = 'out', targetHandle = 'in'): Edge {
  return {
    id: `${source}->${target}:${targetHandle}`,
    source,
    target,
    sourceHandle,
    targetHandle,
  } as Edge
}

// ---------------------------------------------------------------------------
// detectCycles
// ---------------------------------------------------------------------------

describe('detectCycles', () => {
  it('returns [] for empty graph', () => {
    expect(detectCycles([], [])).toEqual([])
  })

  it('returns [] for a single node with no edges', () => {
    expect(detectCycles([makeNode('A')], [])).toEqual([])
  })

  it('returns [] for a simple linear chain A→B→C', () => {
    const nodes = ['A', 'B', 'C'].map((id) => makeNode(id))
    const edges = [makeEdge('A', 'B'), makeEdge('B', 'C')]
    expect(detectCycles(nodes, edges)).toEqual([])
  })

  it('returns [] for a diamond A→B, A→C, B→D, C→D', () => {
    const nodes = ['A', 'B', 'C', 'D'].map((id) => makeNode(id))
    const edges = [makeEdge('A', 'B'), makeEdge('A', 'C'), makeEdge('B', 'D'), makeEdge('C', 'D')]
    expect(detectCycles(nodes, edges)).toEqual([])
  })

  it('detects a simple two-node cycle A→B→A', () => {
    const nodes = ['A', 'B'].map((id) => makeNode(id))
    const edges = [makeEdge('A', 'B'), makeEdge('B', 'A')]
    const cycles = detectCycles(nodes, edges)
    expect(cycles).toHaveLength(1)
    expect(cycles[0]).toContain('A')
    expect(cycles[0]).toContain('B')
  })

  it('detects a three-node cycle A→B→C→A', () => {
    const nodes = ['A', 'B', 'C'].map((id) => makeNode(id))
    const edges = [makeEdge('A', 'B'), makeEdge('B', 'C'), makeEdge('C', 'A')]
    const cycles = detectCycles(nodes, edges)
    expect(cycles).toHaveLength(1)
    expect(cycles[0]).toHaveLength(3)
  })

  it('detects a mid-chain cycle B→C→B with an entry from A', () => {
    const nodes = ['A', 'B', 'C'].map((id) => makeNode(id))
    const edges = [makeEdge('A', 'B'), makeEdge('B', 'C'), makeEdge('C', 'B')]
    const cycles = detectCycles(nodes, edges)
    expect(cycles).toHaveLength(1)
    expect(cycles[0]).toContain('B')
    expect(cycles[0]).toContain('C')
    expect(cycles[0]).not.toContain('A')
  })

  it('detects a self-loop (edge source === target)', () => {
    const nodes = [makeNode('A')]
    // Self-loop: A→A (canvas prevents this but algorithm handles it)
    const edges = [{ id: 'A->A', source: 'A', target: 'A', sourceHandle: 'out', targetHandle: 'in' } as Edge]
    const cycles = detectCycles(nodes, edges)
    expect(cycles).toHaveLength(1)
    expect(cycles[0]).toContain('A')
  })

  it('handles disconnected components — detects cycle only in the cyclic component', () => {
    const nodes = ['A', 'B', 'C', 'D'].map((id) => makeNode(id))
    // A→B is acyclic; C→D→C is cyclic
    const edges = [makeEdge('A', 'B'), makeEdge('C', 'D'), makeEdge('D', 'C')]
    const cycles = detectCycles(nodes, edges)
    expect(cycles).toHaveLength(1)
    const cycleNodes = cycles[0]
    expect(cycleNodes).toContain('C')
    expect(cycleNodes).toContain('D')
  })
})

// ---------------------------------------------------------------------------
// findOrphanedNodes
// ---------------------------------------------------------------------------

describe('findOrphanedNodes', () => {
  it('returns [] for empty graph', () => {
    expect(findOrphanedNodes([], [])).toEqual([])
  })

  it('returns [] for a single isolated node (no edges) — it IS the output', () => {
    // A node with no outgoing edges is an output, so it is reachable from itself
    const nodes = [makeNode('A')]
    expect(findOrphanedNodes(nodes, [])).toEqual([])
  })

  it('returns [] when all nodes are connected to the output', () => {
    const nodes = ['A', 'B', 'C'].map((id) => makeNode(id))
    const edges = [makeEdge('A', 'B'), makeEdge('B', 'C')]
    // C has no outgoing edge → output. B and A are reachable from C via reverse BFS.
    expect(findOrphanedNodes(nodes, edges)).toEqual([])
  })

  it('isolated node (no edges at all) is treated as an output, never orphaned', () => {
    // Nodes with no outgoing edges are sinks → counted as outputs → reachable from themselves.
    // This means a disconnected island is never considered orphaned.
    const nodes = ['A', 'B', 'D'].map((id) => makeNode(id))
    const edges = [makeEdge('A', 'B')] // D has no edges → D is a sink → D is an output
    const orphans = findOrphanedNodes(nodes, edges)
    expect(orphans).not.toContain('D')
    expect(orphans).not.toContain('A')
    expect(orphans).not.toContain('B')
  })

  it('nodes in a cycle (with no external sink) are all orphaned', () => {
    // C→D→E→C is a pure cycle — none of C,D,E are sinks, BFS from sinks never reaches them
    const nodes = ['A', 'B', 'C', 'D', 'E'].map((id) => makeNode(id))
    const edges = [
      makeEdge('A', 'B'), // B is the output for A
      makeEdge('C', 'D'), makeEdge('D', 'E'), makeEdge('E', 'C'), // isolated cycle
    ]
    const orphans = findOrphanedNodes(nodes, edges)
    expect(orphans).toContain('C')
    expect(orphans).toContain('D')
    expect(orphans).toContain('E')
    expect(orphans).not.toContain('A')
    expect(orphans).not.toContain('B')
  })
})

// ---------------------------------------------------------------------------
// validateGraph
// ---------------------------------------------------------------------------

describe('validateGraph', () => {
  it('returns error for empty graph', () => {
    const issues = validateGraph([], [])
    expect(issues.some((i) => i.severity === 'error')).toBe(true)
    expect(issues[0].message).toBe('Graph is empty')
  })

  it('returns no issues for a valid single-node graph', () => {
    // A single variable node with no edges — it IS the output, has no required inputs
    const nodes = [makeNode('A', 'variable')]
    const issues = validateGraph(nodes, [])
    expect(issues.filter((i) => i.severity === 'error')).toHaveLength(0)
  })

  it('returns error when graph has a cycle', () => {
    const nodes = ['A', 'B'].map((id) => makeNode(id, 'variable'))
    const edges = [makeEdge('A', 'B'), makeEdge('B', 'A')]
    const issues = validateGraph(nodes, edges)
    expect(issues.some((i) => i.message.includes('Circular dependency'))).toBe(true)
  })

  it('returns error when no output node exists (all nodes have outgoing edges)', () => {
    const nodes = ['A', 'B'].map((id) => makeNode(id, 'variable'))
    // Both A and B have outgoing edges → no output
    const edges = [makeEdge('A', 'B'), makeEdge('B', 'A')]
    const issues = validateGraph(nodes, edges)
    // Either cycle error or no-output error; at least one error
    expect(issues.some((i) => i.severity === 'error')).toBe(true)
  })

  it('returns error for self-loop edge (source === target)', () => {
    const nodes = [makeNode('A', 'variable')]
    const selfLoop = { id: 'self', source: 'A', target: 'A', sourceHandle: 'out', targetHandle: 'in' } as Edge
    const issues = validateGraph(nodes, [selfLoop])
    expect(issues.some((i) => i.message.includes('cannot connect to itself'))).toBe(true)
  })

  it('returns error for duplicate connection on same port', () => {
    const nodes = ['A', 'B', 'C'].map((id) => makeNode(id, 'variable'))
    // Both A and B connect to C's 'in' port
    const edges = [
      makeEdge('A', 'C', 'out', 'in'),
      makeEdge('B', 'C', 'out', 'in'),
    ]
    const issues = validateGraph(nodes, edges)
    expect(issues.some((i) => i.message.includes('Duplicate connection'))).toBe(true)
  })

  it('orphan warning is suppressed when there are errors', () => {
    const nodes = ['A', 'B', 'C'].map((id) => makeNode(id, 'variable'))
    // A→B cycle (error), C is orphaned — but orphan warning should be suppressed
    const edges = [makeEdge('A', 'B'), makeEdge('B', 'A')]
    const issues = validateGraph(nodes, edges)
    expect(issues.some((i) => i.severity === 'warning')).toBe(false)
    expect(issues.some((i) => i.severity === 'error')).toBe(true)
  })

  it('no orphan warning for disconnected nodes — each isolated node is its own output', () => {
    // Isolated nodes have no outgoing edges → they ARE outputs → findOrphanedNodes skips them.
    // All three nodes are variable leaf nodes with no required inputs and no edges.
    const nodes = ['A', 'B', 'C'].map((id) => makeNode(id, 'variable'))
    const issues = validateGraph(nodes, [])
    expect(issues.filter((i) => i.severity === 'warning')).toHaveLength(0)
    expect(issues.filter((i) => i.severity === 'error')).toHaveLength(0)
  })
})

// ---------------------------------------------------------------------------
// validateGraph — loop node rules
// ---------------------------------------------------------------------------

function makeLoopNode(id: string, config: Record<string, unknown> = {}): Node {
  return {
    id,
    type: 'formulaNode',
    position: { x: 0, y: 0 },
    data: {
      nodeType: 'loop',
      config: {
        mode: 'range',
        formulaId: 'body-formula',
        iterator: 't',
        aggregation: 'sum',
        ...config,
      },
    },
  } as unknown as Node
}

describe('validateGraph — loop node', () => {
  it('passes for a valid loop with start and end connected', () => {
    const loop = makeLoopNode('loop')
    const start = makeNode('c_start', 'constant')
    const end = makeNode('c_end', 'constant')
    const edges = [
      makeEdge('c_start', 'loop', 'out', 'start'),
      makeEdge('c_end', 'loop', 'out', 'end'),
    ]
    const issues = validateGraph([loop, start, end], edges)
    expect(issues.filter((i) => i.severity === 'error')).toHaveLength(0)
  })

  it('error when start port is not connected', () => {
    const loop = makeLoopNode('loop')
    const end = makeNode('c_end', 'constant')
    const edges = [makeEdge('c_end', 'loop', 'out', 'end')]
    const issues = validateGraph([loop, end], edges)
    expect(issues.some((i) => i.message.includes('"start"') || i.message.toLowerCase().includes('start'))).toBe(true)
  })

  it('error when end port is not connected', () => {
    const loop = makeLoopNode('loop')
    const start = makeNode('c_start', 'constant')
    const edges = [makeEdge('c_start', 'loop', 'out', 'start')]
    const issues = validateGraph([loop, start], edges)
    expect(issues.some((i) => i.message.includes('"end"') || i.message.toLowerCase().includes('end'))).toBe(true)
  })

  it('step port is optional — no error when step is not connected', () => {
    const loop = makeLoopNode('loop')
    const start = makeNode('c_start', 'constant')
    const end = makeNode('c_end', 'constant')
    const edges = [
      makeEdge('c_start', 'loop', 'out', 'start'),
      makeEdge('c_end', 'loop', 'out', 'end'),
    ]
    const issues = validateGraph([loop, start, end], edges)
    // No error about step being missing
    expect(issues.filter((i) => i.message.toLowerCase().includes('step'))).toHaveLength(0)
  })

  it('error when formulaId is empty', () => {
    const loop = makeLoopNode('loop', { formulaId: '' })
    const start = makeNode('c_start', 'constant')
    const end = makeNode('c_end', 'constant')
    const edges = [
      makeEdge('c_start', 'loop', 'out', 'start'),
      makeEdge('c_end', 'loop', 'out', 'end'),
    ]
    const issues = validateGraph([loop, start, end], edges)
    expect(issues.some((i) => i.message.toLowerCase().includes('body formula') || i.message.toLowerCase().includes('formula'))).toBe(true)
  })

  it('error when iterator is empty', () => {
    const loop = makeLoopNode('loop', { iterator: '' })
    const start = makeNode('c_start', 'constant')
    const end = makeNode('c_end', 'constant')
    const edges = [
      makeEdge('c_start', 'loop', 'out', 'start'),
      makeEdge('c_end', 'loop', 'out', 'end'),
    ]
    const issues = validateGraph([loop, start, end], edges)
    expect(issues.some((i) => i.message.toLowerCase().includes('iterator'))).toBe(true)
  })

  it('error for invalid aggregation value', () => {
    const loop = makeLoopNode('loop', { aggregation: 'median' })
    const start = makeNode('c_start', 'constant')
    const end = makeNode('c_end', 'constant')
    const edges = [
      makeEdge('c_start', 'loop', 'out', 'start'),
      makeEdge('c_end', 'loop', 'out', 'end'),
    ]
    const issues = validateGraph([loop, start, end], edges)
    expect(issues.some((i) => i.message.includes('median') || i.message.toLowerCase().includes('aggregation'))).toBe(true)
  })

  it('error when aggregation is empty string (falsy bypass)', () => {
    const loop = makeLoopNode('loop', { aggregation: '' })
    const start = makeNode('c_start', 'constant')
    const end = makeNode('c_end', 'constant')
    const edges = [
      makeEdge('c_start', 'loop', 'out', 'start'),
      makeEdge('c_end', 'loop', 'out', 'end'),
    ]
    const issues = validateGraph([loop, start, end], edges)
    expect(issues.some((i) => i.severity === 'error' && i.message.toLowerCase().includes('aggregation'))).toBe(true)
  })

  it('all valid aggregation values are accepted', () => {
    const validAggs = ['sum', 'product', 'count', 'avg', 'min', 'max', 'last']
    for (const agg of validAggs) {
      const loop = makeLoopNode('loop', { aggregation: agg })
      const start = makeNode('c_start', 'constant')
      const end = makeNode('c_end', 'constant')
      const edges = [
        makeEdge('c_start', 'loop', 'out', 'start'),
        makeEdge('c_end', 'loop', 'out', 'end'),
      ]
      const issues = validateGraph([loop, start, end], edges)
      const aggErrors = issues.filter((i) => i.severity === 'error' && i.message.toLowerCase().includes('aggregation'))
      expect(aggErrors).toHaveLength(0)
    }
  })
})
