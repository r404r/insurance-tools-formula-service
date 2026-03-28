import type { Edge, Node } from '@xyflow/react'

function operatorSymbol(op: string): string {
  switch (op) {
    case 'add':
      return '+'
    case 'subtract':
      return '-'
    case 'multiply':
      return '*'
    case 'divide':
      return '/'
    case 'power':
      return '^'
    case 'modulo':
      return '%'
    default:
      return op
  }
}

function comparatorSymbol(comparator: string): string {
  switch (comparator) {
    case 'gt':
      return '>'
    case 'lt':
      return '<'
    case 'ge':
      return '>='
    case 'le':
      return '<='
    case 'eq':
      return '=='
    case 'ne':
      return '!='
    default:
      return comparator
  }
}

function incomingSource(edges: Edge[], target: string, targetHandle: string): string | null {
  const edge = edges.find((item) => item.target === target && item.targetHandle === targetHandle)
  return edge?.source ?? null
}

function firstIncomingSource(edges: Edge[], target: string): string | null {
  const edge = edges.find((item) => item.target === target)
  return edge?.source ?? null
}

function isIdentifier(value: string): boolean {
  return /^[A-Za-z_][A-Za-z0-9_]*$/.test(value)
}

function quoteIfNeeded(value: string): string {
  return isIdentifier(value) ? value : JSON.stringify(value)
}

export function reactFlowToText(nodes: Node[], edges: Edge[]): string {
  if (nodes.length === 0) {
    return ''
  }

  const nodeMap = new Map(nodes.map((node) => [node.id, node]))
  const cache = new Map<string, string>()

  const renderNode = (nodeId: string, stack = new Set<string>()): string => {
    const cached = cache.get(nodeId)
    if (cached) {
      return cached
    }

    if (stack.has(nodeId)) {
      throw new Error(`Cycle detected while rendering node ${nodeId}`)
    }

    const node = nodeMap.get(nodeId)
    if (!node) {
      throw new Error(`Node ${nodeId} not found`)
    }

    const nextStack = new Set(stack)
    nextStack.add(nodeId)

    const nodeType = String(node.data.nodeType ?? node.type)
    const config = (node.data.config as Record<string, unknown>) ?? {}

    let result = ''

    switch (nodeType) {
      case 'variable':
        result = String(config.name ?? nodeId)
        break
      case 'constant':
        result = String(config.value ?? '0')
        break
      case 'operator': {
        const leftId = incomingSource(edges, nodeId, 'left')
        const rightId = incomingSource(edges, nodeId, 'right')
        if (!leftId || !rightId) {
          throw new Error(`Operator node ${nodeId} is missing left or right input`)
        }
        result = `(${renderNode(leftId, nextStack)} ${operatorSymbol(String(config.op ?? '?'))} ${renderNode(rightId, nextStack)})`
        break
      }
      case 'function': {
        const fn = String(config.fn ?? 'fn')
        const args = (config.args as Record<string, string> | undefined) ?? {}
        if (fn === 'min' || fn === 'max') {
          const leftId = incomingSource(edges, nodeId, 'left')
          const rightId = incomingSource(edges, nodeId, 'right')
          if (!leftId || !rightId) {
            throw new Error(`Function node ${nodeId} is missing left or right input`)
          }
          result = `${fn}(${renderNode(leftId, nextStack)}, ${renderNode(rightId, nextStack)})`
          break
        }

        const inId = incomingSource(edges, nodeId, 'in') ?? incomingSource(edges, nodeId, 'arg0') ?? firstIncomingSource(edges, nodeId)
        if (!inId) {
          throw new Error(`Function node ${nodeId} is missing input`)
        }

        if (fn === 'round' && args.places) {
          result = `${fn}(${renderNode(inId, nextStack)}, ${args.places})`
        } else {
          result = `${fn}(${renderNode(inId, nextStack)})`
        }
        break
      }
      case 'tableLookup': {
        const keyId = incomingSource(edges, nodeId, 'key')
        if (!keyId) {
          throw new Error(`Table lookup node ${nodeId} is missing key input`)
        }
        const tableId = String(config.tableId ?? '')
        const column = String(config.column ?? '')
        const parts = [quoteIfNeeded(tableId), renderNode(keyId, nextStack)]
        if (column) {
          parts.push(quoteIfNeeded(column))
        }
        result = `lookup(${parts.join(', ')})`
        break
      }
      case 'conditional': {
        const conditionId = incomingSource(edges, nodeId, 'condition')
        const conditionRightId = incomingSource(edges, nodeId, 'conditionRight')
        const thenId = incomingSource(edges, nodeId, 'thenValue')
        const elseId = incomingSource(edges, nodeId, 'elseValue')
        if (!conditionId || !conditionRightId || !thenId || !elseId) {
          throw new Error(`Conditional node ${nodeId} is missing one or more inputs`)
        }
        const comparator = comparatorSymbol(String(config.comparator ?? '=='))
        result = `if ${renderNode(conditionId, nextStack)} ${comparator} ${renderNode(conditionRightId, nextStack)} then ${renderNode(thenId, nextStack)} else ${renderNode(elseId, nextStack)}`
        break
      }
      case 'aggregate': {
        const itemsId = incomingSource(edges, nodeId, 'items')
        if (!itemsId) {
          throw new Error(`Aggregate node ${nodeId} is missing items input`)
        }
        result = `${String(config.fn ?? 'sum')}(${renderNode(itemsId, nextStack)})`
        break
      }
      case 'subFormula': {
        const inputId = incomingSource(edges, nodeId, 'in')
        const formulaId = String(config.formulaId ?? '')
        result = inputId
          ? `subFormula(${quoteIfNeeded(formulaId)}, ${renderNode(inputId, nextStack)})`
          : `subFormula(${quoteIfNeeded(formulaId)})`
        break
      }
      default:
        throw new Error(`Unsupported node type ${nodeType}`)
    }

    cache.set(nodeId, result)
    return result
  }

  const outputNodes = nodes.filter((node) => edges.every((edge) => edge.source !== node.id))
  if (outputNodes.length === 0) {
    return ''
  }

  return outputNodes
    .map((node, index) => {
      const expression = renderNode(node.id)
      return outputNodes.length > 1 ? `output${index + 1} = ${expression}` : expression
    })
    .join('\n')
}
