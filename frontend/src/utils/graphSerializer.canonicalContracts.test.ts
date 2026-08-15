import { describe, expect, it } from 'vitest'
import type { FormulaGraph } from '../types/formula'
import { apiToReactFlow, reactFlowToApi } from './graphSerializer'
import { reactFlowToText } from './graphText'
import { validateGraph } from './graphValidation'

function assertCanonicalGraphContract(graph: FormulaGraph, expectedText: string) {
  const flow = apiToReactFlow(graph)
  let text: string | Error
  try {
    text = reactFlowToText(flow.nodes, flow.edges)
  } catch (error) {
    text = error as Error
  }
  const serialized = reactFlowToApi(flow.nodes, flow.edges, graph.outputs)

  // A graph created by the parser must remain editable in the visual editor:
  // every canonical backend input port has to be a valid, connectable port.
  expect.soft(validateGraph(flow.nodes, flow.edges).filter((issue) => issue.severity === 'error')).toEqual([])
  expect.soft(text).toBe(expectedText)
  expect.soft(serialized.nodes).toEqual(graph.nodes)
  expect.soft(serialized.edges).toEqual(graph.edges)
  expect.soft(serialized.outputs).toEqual(graph.outputs)
}

describe('canonical parser graph contracts in the visual editor', () => {
  it('keeps unary negate as a one-input operator through validate, text rendering, and API round-trip', () => {
    const graph: FormulaGraph = {
      nodes: [
        { id: 'constant_1', type: 'constant', config: { value: '4' } },
        { id: 'unary_2', type: 'operator', config: { op: 'negate' } },
      ],
      edges: [
        { source: 'constant_1', target: 'unary_2', sourcePort: 'out', targetPort: 'left' },
      ],
      outputs: ['unary_2'],
      layout: { positions: { constant_1: { x: 0, y: 0 }, unary_2: { x: 160, y: 0 } } },
    }

    assertCanonicalGraphContract(graph, '-4')
  })

  it('keeps a standalone comparison as a two-input expression rather than requiring if/then/else branches', () => {
    const graph: FormulaGraph = {
      nodes: [
        { id: 'age', type: 'variable', config: { name: 'age', dataType: 'integer' } },
        { id: 'constant_1', type: 'constant', config: { value: '18' } },
        { id: 'comparison_2', type: 'conditional', config: { comparator: 'ge' } },
      ],
      edges: [
        { source: 'age', target: 'comparison_2', sourcePort: 'out', targetPort: 'condition' },
        { source: 'constant_1', target: 'comparison_2', sourcePort: 'out', targetPort: 'conditionRight' },
      ],
      outputs: ['comparison_2'],
      layout: {
        positions: {
          age: { x: 0, y: 0 },
          constant_1: { x: 0, y: 120 },
          comparison_2: { x: 180, y: 60 },
        },
      },
    }

    assertCanonicalGraphContract(graph, 'age >= 18')
  })

  it('preserves all variadic aggregate items:N ports in parser order and treats them as valid dynamic inputs', () => {
    const graph: FormulaGraph = {
      nodes: [
        { id: 'constant_1', type: 'constant', config: { value: '1' } },
        { id: 'constant_2', type: 'constant', config: { value: '2' } },
        { id: 'constant_3', type: 'constant', config: { value: '3' } },
        { id: 'aggregate_4', type: 'aggregate', config: { fn: 'sum' } },
      ],
      edges: [
        { source: 'constant_1', target: 'aggregate_4', sourcePort: 'out', targetPort: 'items:0' },
        { source: 'constant_2', target: 'aggregate_4', sourcePort: 'out', targetPort: 'items:1' },
        { source: 'constant_3', target: 'aggregate_4', sourcePort: 'out', targetPort: 'items:2' },
      ],
      outputs: ['aggregate_4'],
      layout: {
        positions: {
          constant_1: { x: 0, y: 0 },
          constant_2: { x: 0, y: 100 },
          constant_3: { x: 0, y: 200 },
          aggregate_4: { x: 180, y: 100 },
        },
      },
    }

    assertCanonicalGraphContract(graph, 'sum(1, 2, 3)')
  })
})

describe('floor and ceil precision graph contracts', () => {
  it.each([
    ['floor', 'floor(123.456, 2)'],
    ['ceil', 'ceil(123.456, 2)'],
  ] as const)('renders %s places=2 in the text representation without dropping its persisted config', (fn, expectedText) => {
    const graph: FormulaGraph = {
      nodes: [
        { id: 'constant_1', type: 'constant', config: { value: '123.456' } },
        { id: 'function_2', type: 'function', config: { fn, args: { places: '2' } } },
      ],
      edges: [
        { source: 'constant_1', target: 'function_2', sourcePort: 'out', targetPort: 'in' },
      ],
      outputs: ['function_2'],
      layout: { positions: { constant_1: { x: 0, y: 0 }, function_2: { x: 160, y: 0 } } },
    }

    assertCanonicalGraphContract(graph, expectedText)
  })
})
