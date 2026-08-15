import { describe, expect, it } from 'vitest'
import type { Edge, Node } from '@xyflow/react'
import type { FormulaGraph } from '../types/formula'
import { apiToReactFlow, reactFlowToApi } from './graphSerializer'
import { validateGraph } from './graphValidation'

const dynamicPort = 'dev_year'

function tableAggregateGraph(): FormulaGraph {
  return {
    nodes: [
      {
        id: 'var_dev_year',
        type: 'variable',
        config: { name: 'dev_year', dataType: 'decimal' },
      },
      {
        id: 'agg_ldf',
        // `tableAggregate` is accepted by the backend today. The cast makes
        // this executable red test while the frontend's NodeType contract is
        // corrected by the implementation step.
        type: 'tableAggregate' as FormulaGraph['nodes'][number]['type'],
        config: {
          tableId: '{{table:claims_triangle_sample}}',
          aggregate: 'avg',
          expression: 'development_ratio',
          filters: [
            { column: 'dev_year', op: 'eq', inputPort: dynamicPort },
          ],
        },
      },
    ],
    edges: [
      {
        source: 'var_dev_year',
        target: 'agg_ldf',
        sourcePort: 'out',
        targetPort: dynamicPort,
      },
    ],
    outputs: ['agg_ldf'],
    layout: {
      positions: {
        var_dev_year: { x: 10, y: 20 },
        agg_ldf: { x: 220, y: 20 },
      },
    },
  }
}

describe('tableAggregate API ↔ React Flow contract', () => {
  it('loads, validates, and re-saves the Chain Ladder seed graph without changing its dynamic port', () => {
    const graph = tableAggregateGraph()

    const flow = apiToReactFlow(graph)

    expect(flow.edges).toHaveLength(1)
    expect(flow.edges[0]?.targetHandle).toBe(dynamicPort)
    expect(validateGraph(flow.nodes, flow.edges).filter((issue) => issue.severity === 'error')).toEqual([])

    const roundTripped = reactFlowToApi(
      flow.nodes,
      flow.edges,
      graph.outputs,
    )
    expect(roundTripped.edges).toEqual([
      {
        source: 'var_dev_year',
        target: 'agg_ldf',
        sourcePort: 'out',
        targetPort: dynamicPort,
      },
    ])
  })

  it('restores the configured dynamic filter port when a legacy edge omitted targetPort', () => {
    const graph = tableAggregateGraph()
    graph.edges[0]!.targetPort = ''

    const flow = apiToReactFlow(graph)

    expect(flow.edges[0]?.targetHandle).toBe(dynamicPort)
  })

  it('rejects an aggregate whose dynamic filter input is not connected', () => {
    const graph = tableAggregateGraph()
    const flow = apiToReactFlow(graph)
    const aggregate = flow.nodes.find((node) => node.id === 'agg_ldf')!

    const issues = validateGraph([aggregate] as Node[], [] as Edge[])

    expect(issues).toContainEqual({
      message: `Table aggregate filter must have "${dynamicPort}" input connected`,
      nodeIds: ['agg_ldf'],
      severity: 'error',
    })
  })
})
