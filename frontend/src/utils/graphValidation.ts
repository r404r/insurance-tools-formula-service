import type { Node, Edge } from '@xyflow/react'
import { getInputPorts } from '../components/editor/nodePresentation'

const VALID_LOOP_AGGREGATIONS = new Set(['sum', 'product', 'count', 'avg', 'min', 'max', 'last', 'fold'])

export interface ValidationIssue {
  message: string
  nodeIds: string[]
  severity: 'error' | 'warning'
}

/** Detect cycles in the graph using iterative DFS tri-color marking. Returns arrays of node IDs in each cycle. */
export function detectCycles(nodes: Node[], edges: Edge[]): string[][] {
  const adj = new Map<string, string[]>()
  for (const node of nodes) adj.set(node.id, [])
  for (const edge of edges) {
    const targets = adj.get(edge.source)
    if (targets) targets.push(edge.target)
  }

  const color = new Map<string, 'white' | 'gray' | 'black'>()
  for (const node of nodes) color.set(node.id, 'white')

  const cycles: string[][] = []

  for (const startNode of nodes) {
    if (color.get(startNode.id) !== 'white') continue

    // Each stack frame: [nodeId, neighborIndex, path-so-far]
    // neighborIndex tracks which neighbor we're currently visiting
    const stack: Array<{ nodeId: string; neighborIdx: number; path: string[] }> = []
    stack.push({ nodeId: startNode.id, neighborIdx: 0, path: [startNode.id] })
    color.set(startNode.id, 'gray')

    while (stack.length > 0) {
      const frame = stack[stack.length - 1]
      const neighbors = adj.get(frame.nodeId) ?? []

      if (frame.neighborIdx < neighbors.length) {
        const neighbor = neighbors[frame.neighborIdx]
        frame.neighborIdx++

        if (color.get(neighbor) === 'gray') {
          // Back edge: cycle found
          const cycleStart = frame.path.indexOf(neighbor)
          if (cycleStart !== -1) cycles.push([...frame.path.slice(cycleStart)])
        } else if (color.get(neighbor) === 'white') {
          color.set(neighbor, 'gray')
          stack.push({ nodeId: neighbor, neighborIdx: 0, path: [...frame.path, neighbor] })
        }
      } else {
        // All neighbors visited, backtrack
        color.set(frame.nodeId, 'black')
        stack.pop()
      }
    }
  }

  return cycles
}

/** Find nodes not reachable backwards from any output node (orphaned / dead-code nodes). */
export function findOrphanedNodes(nodes: Node[], edges: Edge[]): string[] {
  // Output nodes: nodes with no outgoing edges — O(V+E) with a pre-built source set
  const sourceIds = new Set(edges.map((e) => e.source))
  const outputNodeIds = new Set(nodes.filter((n) => !sourceIds.has(n.id)).map((n) => n.id))

  // Reverse adjacency: target → predecessors
  const reverseAdj = new Map<string, string[]>()
  for (const node of nodes) reverseAdj.set(node.id, [])
  for (const edge of edges) {
    const preds = reverseAdj.get(edge.target)
    if (preds) preds.push(edge.source)
  }

  // BFS backwards from outputs
  const reachable = new Set<string>(outputNodeIds)
  const queue = [...outputNodeIds]
  while (queue.length > 0) {
    const nodeId = queue.shift()!
    for (const pred of (reverseAdj.get(nodeId) ?? [])) {
      if (!reachable.has(pred)) {
        reachable.add(pred)
        queue.push(pred)
      }
    }
  }

  return nodes.filter((n) => !reachable.has(n.id)).map((n) => n.id)
}

export function validateGraph(nodes: Node[], edges: Edge[]): ValidationIssue[] {
  const issues: ValidationIssue[] = []

  if (nodes.length === 0) {
    issues.push({ message: 'Graph is empty', nodeIds: [], severity: 'error' })
    return issues
  }

  // Edge integrity checks
  const connectedPorts = new Map<string, Set<string>>()
  for (const edge of edges) {
    if (!edge.source || !edge.target) {
      issues.push({ message: 'Edge is missing source or target', nodeIds: [], severity: 'error' })
      continue
    }
    if (!edge.sourceHandle) {
      issues.push({ message: `Edge from ${edge.source} is missing source port`, nodeIds: [edge.source], severity: 'error' })
      continue
    }
    if (!edge.targetHandle) {
      issues.push({ message: `Edge into ${edge.target} is missing target port`, nodeIds: [edge.target], severity: 'error' })
      continue
    }
    if (edge.source === edge.target) {
      issues.push({ message: `Node cannot connect to itself`, nodeIds: [edge.source], severity: 'error' })
      continue
    }

    const ports = connectedPorts.get(edge.target) ?? new Set<string>()
    if (ports.has(edge.targetHandle)) {
      issues.push({ message: `Duplicate connection on port ${edge.targetHandle}`, nodeIds: [edge.target], severity: 'error' })
    }
    ports.add(edge.targetHandle)
    connectedPorts.set(edge.target, ports)
  }

  // Cycle detection
  const cycles = detectCycles(nodes, edges)
  for (const cycle of cycles) {
    issues.push({
      message: `Circular dependency detected (${cycle.length} nodes)`,
      nodeIds: cycle,
      severity: 'error',
    })
  }

  // Per-node validation
  for (const node of nodes) {
    const nodeType = String(node.data.nodeType ?? node.type)
    const config = (node.data.config as Record<string, unknown>) ?? {}
    const ports = connectedPorts.get(node.id) ?? new Set<string>()
    const validTargetPorts = new Set(getInputPorts(nodeType, config).map((port) => port.id))

    for (const port of ports) {
      if (!validTargetPorts.has(port)) {
        issues.push({ message: `Invalid input port: ${port}`, nodeIds: [node.id], severity: 'error' })
      }
    }

    switch (nodeType) {
      case 'operator':
        if (!ports.has('left') || !ports.has('right'))
          issues.push({ message: 'Operator must have left and right inputs', nodeIds: [node.id], severity: 'error' })
        break
      case 'function':
        if (config.fn === 'min' || config.fn === 'max') {
          if (!ports.has('left') || !ports.has('right'))
            issues.push({ message: 'Function min/max must have left and right inputs', nodeIds: [node.id], severity: 'error' })
        } else if (!ports.has('in')) {
          issues.push({ message: 'Function must have an in input', nodeIds: [node.id], severity: 'error' })
        }
        break
      case 'subFormula':
        if (!String(config.formulaId ?? '').trim())
          issues.push({ message: 'Sub-formula must reference a formula', nodeIds: [node.id], severity: 'error' })
        break
      case 'tableLookup': {
        const kcs = (config.keyColumns as string[] | undefined) ?? ['key']
        for (const kc of kcs) {
          if (!kc.trim())
            issues.push({ message: 'Table lookup has an empty key column name', nodeIds: [node.id], severity: 'error' })
          else if (!ports.has(kc))
            issues.push({ message: `Table lookup must have "${kc}" input connected`, nodeIds: [node.id], severity: 'error' })
        }
        break
      }
      case 'conditional':
        for (const port of ['condition', 'conditionRight', 'thenValue', 'elseValue']) {
          if (!ports.has(port))
            issues.push({ message: `Conditional must have "${port}" input`, nodeIds: [node.id], severity: 'error' })
        }
        break
      case 'aggregate':
        if (!ports.has('items'))
          issues.push({ message: 'Aggregate must have an items input', nodeIds: [node.id], severity: 'error' })
        break
      case 'loop': {
        const loopCfg = config as { formulaId?: string; iterator?: string; aggregation?: string; mode?: string; accumulatorVar?: string }
        if (!String(loopCfg.formulaId ?? '').trim())
          issues.push({ message: 'Loop must reference a body formula', nodeIds: [node.id], severity: 'error' })
        if (!String(loopCfg.iterator ?? '').trim())
          issues.push({ message: 'Loop must have an iterator variable name', nodeIds: [node.id], severity: 'error' })
        else if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(String(loopCfg.iterator ?? '')))
          issues.push({ message: `Loop iterator "${loopCfg.iterator}" must be a valid identifier (letters, digits, underscores)`, nodeIds: [node.id], severity: 'error' })
        if (!loopCfg.aggregation || !VALID_LOOP_AGGREGATIONS.has(loopCfg.aggregation))
          issues.push({ message: `Loop has invalid aggregation "${loopCfg.aggregation ?? ''}"`, nodeIds: [node.id], severity: 'error' })
        if (loopCfg.aggregation === 'fold' && !String(loopCfg.accumulatorVar ?? '').trim())
          issues.push({ message: 'Fold loop must have an accumulator variable name', nodeIds: [node.id], severity: 'error' })
        if (loopCfg.mode && loopCfg.mode !== 'range')
          issues.push({ message: `Loop mode must be "range", got "${loopCfg.mode}"`, nodeIds: [node.id], severity: 'error' })
        if (!ports.has('start'))
          issues.push({ message: 'Loop must have "start" input connected', nodeIds: [node.id], severity: 'error' })
        if (!ports.has('end'))
          issues.push({ message: 'Loop must have "end" input connected', nodeIds: [node.id], severity: 'error' })
        // 'step' is optional — no error if absent.
        break
      }
    }
  }

  const edgeSourceIds = new Set(edges.map((e) => e.source))
  const outputNodes = nodes.filter((n) => !edgeSourceIds.has(n.id))
  if (outputNodes.length === 0) {
    issues.push({ message: 'Graph must contain at least one output node', nodeIds: [], severity: 'error' })
  }

  // Orphaned node warning (only when no errors so far, to avoid noise)
  if (issues.every((i) => i.severity !== 'error')) {
    const orphaned = findOrphanedNodes(nodes, edges)
    if (orphaned.length > 0) {
      issues.push({
        message: `${orphaned.length} node(s) are not connected to any output`,
        nodeIds: orphaned,
        severity: 'warning',
      })
    }
  }

  return issues
}
