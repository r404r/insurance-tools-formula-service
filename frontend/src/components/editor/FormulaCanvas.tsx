import { useCallback, useRef, type Dispatch, type SetStateAction } from 'react'
import { ValidationContext, type ValidationState } from './ValidationContext'
import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  MarkerType,
  addEdge,
  applyNodeChanges,
  applyEdgeChanges,
  type Node,
  type Edge,
  type OnNodesChange,
  type OnEdgesChange,
  type OnConnect,
  type Connection,
  type IsValidConnection,
  type ReactFlowInstance,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { useAutoLayout } from './hooks/useAutoLayout'
import type { NodeType } from '../../types/formula'
import FormulaNode from './FormulaNode'
import { createNodeData, defaultNodeConfig, getInputPorts } from './nodePresentation'

interface Props {
  nodes: Node[]
  edges: Edge[]
  onNodesChange: Dispatch<SetStateAction<Node[]>>
  onEdgesChange: Dispatch<SetStateAction<Edge[]>>
  onNodeSelect: (node: Node | null) => void
  validation?: ValidationState
}

let idCounter = 0
function nextId() {
  return `node_${Date.now()}_${idCounter++}`
}

const nodeTypes = {
  formulaNode: FormulaNode,
}

const emptyValidation: ValidationState = { invalidNodeIds: new Set(), warnNodeIds: new Set() }

function dynamicInputPorts(node: Node): string[] {
  const ports = (node.data as Record<string, unknown>).inputPorts
  return Array.isArray(ports)
    ? ports.filter((port): port is string => typeof port === 'string')
    : []
}

function withNextAggregateInputPort(node: Node, connectedPort: string): Node {
  const nodeType = String(node.data.nodeType ?? node.type)
  if (nodeType !== 'aggregate' || !/^items:\d+$/.test(connectedPort)) return node

  const existing = dynamicInputPorts(node)
  const indexes = [...existing, connectedPort]
    .filter((port) => /^items:\d+$/.test(port))
    .map((port) => Number(port.slice('items:'.length)))
  const nextPort = `items:${Math.max(...indexes) + 1}`
  const inputPorts = [...new Set([...existing, connectedPort, nextPort])].sort((left, right) => {
    const leftIndex = Number(left.slice('items:'.length))
    const rightIndex = Number(right.slice('items:'.length))
    return leftIndex - rightIndex
  })

  return { ...node, data: { ...node.data, inputPorts } }
}

export default function FormulaCanvas({ nodes, edges, onNodesChange, onEdgesChange, onNodeSelect, validation }: Props) {
  const autoLayout = useAutoLayout()
  const rfInstance = useRef<ReactFlowInstance | null>(null)

  const isValidConnection: IsValidConnection = useCallback(
    (connection: Connection | Edge) => {
      if (!connection.source || !connection.target || !connection.sourceHandle || !connection.targetHandle) {
        return false
      }

      if (connection.source === connection.target) {
        return false
      }

      if (connection.sourceHandle !== 'out') {
        return false
      }

      const targetNode = nodes.find((node) => node.id === connection.target)
      if (!targetNode) {
        return false
      }

      const config = (targetNode.data.config as Record<string, unknown>) ?? {}
      const validTargetPorts = new Set(
        getInputPorts(
          String(targetNode.data.nodeType ?? targetNode.type),
          config,
          dynamicInputPorts(targetNode),
        ).map((port) => port.id),
      )
      if (!validTargetPorts.has(connection.targetHandle)) {
        return false
      }

      const targetPortAlreadyUsed = edges.some(
        (edge) =>
          edge.target === connection.target &&
          edge.targetHandle === connection.targetHandle
      )
      if (targetPortAlreadyUsed) {
        return false
      }

      const duplicateEdge = edges.some(
        (edge) =>
          edge.source === connection.source &&
          edge.target === connection.target &&
          edge.sourceHandle === connection.sourceHandle &&
          edge.targetHandle === connection.targetHandle
      )
      return !duplicateEdge
    },
    [edges, nodes]
  )

  const handleNodesChange: OnNodesChange = useCallback(
    (changes) => {
      onNodesChange((prev) => applyNodeChanges(changes, prev))
    },
    [onNodesChange]
  )

  const handleEdgesChange: OnEdgesChange = useCallback(
    (changes) => {
      onEdgesChange((prev) => applyEdgeChanges(changes, prev))
    },
    [onEdgesChange]
  )

  const handleConnect: OnConnect = useCallback(
    (params) => {
      if (!isValidConnection(params)) {
        return
      }
      onEdgesChange((prev) =>
        addEdge(
          {
            ...params,
            id: `edge_${Date.now()}`,
            style: { stroke: '#64748b', strokeWidth: 2 },
            markerEnd: { type: MarkerType.ArrowClosed, color: '#64748b' },
          },
          prev
        )
      )
      if (params.target && params.targetHandle) {
        onNodesChange((prev) =>
          prev.map((node) => node.id === params.target
            ? withNextAggregateInputPort(node, params.targetHandle!)
            : node),
        )
      }
    },
    [isValidConnection, onEdgesChange, onNodesChange]
  )

  const handleNodeClick = useCallback(
    (_: React.MouseEvent, node: Node) => {
      onNodeSelect(node)
    },
    [onNodeSelect]
  )

  const handlePaneClick = useCallback(() => {
    onNodeSelect(null)
  }, [onNodeSelect])

  const handleDrop = useCallback(
    (event: React.DragEvent) => {
      event.preventDefault()
      const type = event.dataTransfer.getData('application/reactflow-type') as NodeType
      if (!type) return

      if (!rfInstance.current) return

      const position = rfInstance.current.screenToFlowPosition({
        x: event.clientX,
        y: event.clientY,
      })

      const newNode: Node = {
        id: nextId(),
        type: 'formulaNode',
        position,
        data: createNodeData(type, defaultNodeConfig(type)),
      }

      // Route through applyNodeChanges with an 'add' change so React Flow's
      // internal state stays in sync with user state. Using a functional
      // updater ensures we always operate on the latest nodes snapshot,
      // avoiding stale-closure races with other NodeChange events that fire
      // right after the drop (e.g. dimension changes from ResizeObserver).
      onNodesChange((prev) => applyNodeChanges([{ type: 'add', item: newNode }], prev))
    },
    [onNodesChange]
  )

  const handleDragOver = useCallback((event: React.DragEvent) => {
    event.preventDefault()
    event.dataTransfer.dropEffect = 'move'
  }, [])

  const handleAutoLayout = useCallback(() => {
    const laid = autoLayout(nodes, edges)
    onNodesChange(laid)
    // Re-fit after layout so the graph is fully visible
    requestAnimationFrame(() => {
      rfInstance.current?.fitView({ padding: 0.15 })
    })
  }, [nodes, edges, autoLayout, onNodesChange])

  return (
    <ValidationContext.Provider value={validation ?? emptyValidation}>
    <div className="relative flex-1 h-full min-h-[400px]">
      <div className="pointer-events-none absolute top-2 right-2 z-10">
        <button
          onClick={handleAutoLayout}
          className="pointer-events-auto bg-white border border-gray-300 rounded px-3 py-1 text-xs hover:bg-gray-50 shadow-sm"
        >
          Auto Layout
        </button>
      </div>
      <ReactFlow
        nodeTypes={nodeTypes}
        nodes={nodes}
        edges={edges}
        isValidConnection={isValidConnection}
        onNodesChange={handleNodesChange}
        onEdgesChange={handleEdgesChange}
        onConnect={handleConnect}
        onNodeClick={handleNodeClick}
        onPaneClick={handlePaneClick}
        onDrop={handleDrop}
        onDragOver={handleDragOver}
        onInit={(instance) => { rfInstance.current = instance }}
        fitView
      >
        <Background />
        <Controls />
        <MiniMap pannable zoomable />
      </ReactFlow>
    </div>
    </ValidationContext.Provider>
  )
}
