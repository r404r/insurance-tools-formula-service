import { useCallback, useState } from 'react'
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
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { useAutoLayout } from './hooks/useAutoLayout'
import type { NodeType } from '../../types/formula'

interface Props {
  nodes: Node[]
  edges: Edge[]
  onNodesChange: (nodes: Node[]) => void
  onEdgesChange: (edges: Edge[]) => void
  onNodeSelect: (node: Node | null) => void
}

let idCounter = 0
function nextId() {
  return `node_${Date.now()}_${idCounter++}`
}

export default function FormulaCanvas({ nodes, edges, onNodesChange, onEdgesChange, onNodeSelect }: Props) {
  const [selectedNode, setSelectedNode] = useState<string | null>(null)
  const autoLayout = useAutoLayout()

  const handleNodesChange: OnNodesChange = useCallback(
    (changes) => {
      const updated = applyNodeChanges(changes, nodes)
      onNodesChange(updated)
    },
    [nodes, onNodesChange]
  )

  const handleEdgesChange: OnEdgesChange = useCallback(
    (changes) => {
      const updated = applyEdgeChanges(changes, edges)
      onEdgesChange(updated)
    },
    [edges, onEdgesChange]
  )

  const handleConnect: OnConnect = useCallback(
    (params) => {
      const updated = addEdge({
        ...params,
        id: `edge_${Date.now()}`,
        style: { stroke: '#64748b', strokeWidth: 2 },
        markerEnd: { type: MarkerType.ArrowClosed, color: '#64748b' },
      }, edges)
      onEdgesChange(updated)
    },
    [edges, onEdgesChange]
  )

  const handleNodeClick = useCallback(
    (_: React.MouseEvent, node: Node) => {
      setSelectedNode(node.id)
      onNodeSelect(node)
    },
    [onNodeSelect]
  )

  const handlePaneClick = useCallback(() => {
    setSelectedNode(null)
    onNodeSelect(null)
  }, [onNodeSelect])

  const handleDrop = useCallback(
    (event: React.DragEvent) => {
      event.preventDefault()
      const type = event.dataTransfer.getData('application/reactflow-type') as NodeType
      if (!type) return

      const bounds = (event.target as HTMLElement).closest('.react-flow')?.getBoundingClientRect()
      if (!bounds) return

      const position = {
        x: event.clientX - bounds.left,
        y: event.clientY - bounds.top,
      }

      const NODE_COLORS: Record<string, { bg: string; border: string }> = {
        variable:    { bg: '#dbeafe', border: '#3b82f6' },
        constant:    { bg: '#fef3c7', border: '#f59e0b' },
        operator:    { bg: '#fce7f3', border: '#ec4899' },
        function:    { bg: '#d1fae5', border: '#10b981' },
        subFormula:  { bg: '#e0e7ff', border: '#6366f1' },
        tableLookup: { bg: '#fae8ff', border: '#a855f7' },
        conditional: { bg: '#ffedd5', border: '#f97316' },
        aggregate:   { bg: '#ccfbf1', border: '#14b8a6' },
      }
      const colors = NODE_COLORS[type] ?? { bg: '#f3f4f6', border: '#9ca3af' }

      const newNode: Node = {
        id: nextId(),
        type: 'default',
        position,
        data: { label: type, nodeType: type, config: {} },
        style: {
          background: colors.bg,
          border: `2px solid ${colors.border}`,
          borderRadius: 8,
          fontSize: 13,
          fontWeight: 600,
          padding: '4px 8px',
          minWidth: 60,
          textAlign: 'center' as const,
        },
      }

      onNodesChange([...nodes, newNode])
    },
    [nodes, onNodesChange]
  )

  const handleDragOver = useCallback((event: React.DragEvent) => {
    event.preventDefault()
    event.dataTransfer.dropEffect = 'move'
  }, [])

  const handleAutoLayout = useCallback(() => {
    const laid = autoLayout(nodes, edges)
    onNodesChange(laid)
  }, [nodes, edges, autoLayout, onNodesChange])

  return (
    <div className="flex-1 relative">
      <div className="absolute top-2 right-2 z-10">
        <button
          onClick={handleAutoLayout}
          className="bg-white border border-gray-300 rounded px-3 py-1 text-xs hover:bg-gray-50 shadow-sm"
        >
          Auto Layout
        </button>
      </div>
      <ReactFlow
        nodes={nodes.map((n) => ({
          ...n,
          style: {
            ...n.style,
            ...(n.id === selectedNode ? { boxShadow: '0 0 0 3px #3b82f6' } : {}),
          },
        }))}
        edges={edges}
        onNodesChange={handleNodesChange}
        onEdgesChange={handleEdgesChange}
        onConnect={handleConnect}
        onNodeClick={handleNodeClick}
        onPaneClick={handlePaneClick}
        onDrop={handleDrop}
        onDragOver={handleDragOver}
        fitView
      >
        <Background />
        <Controls />
        <MiniMap />
      </ReactFlow>
    </div>
  )
}
