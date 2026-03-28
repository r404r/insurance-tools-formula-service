import { useEffect, useState, useCallback } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import type { Node, Edge } from '@xyflow/react'
import { useFormulaStore } from '../../store/formulaStore'
import { api } from '../../api/client'
import { apiToReactFlow, reactFlowToApi } from '../../utils/graphSerializer'
import FormulaCanvas from './FormulaCanvas'
import TextEditor from './TextEditor'
import NodePalette from './NodePalette'
import NodePropertiesPanel from './NodePropertiesPanel'
import type { Formula, FormulaVersion, NodeType } from '../../types/formula'
import { createNodeData, getInputPorts } from './nodePresentation'

function validateGraph(nodes: Node[], edges: Edge[]): string | null {
  if (nodes.length === 0) return 'Graph is empty'

  const connectedPorts = new Map<string, Set<string>>()
  for (const edge of edges) {
    if (!edge.source || !edge.target) return 'Edge is missing source or target'
    if (!edge.sourceHandle) return `Edge from ${edge.source} is missing source port`
    if (!edge.targetHandle) return `Edge into ${edge.target} is missing target port`
    if (edge.source === edge.target) return `Node ${edge.source} cannot connect to itself`

    const ports = connectedPorts.get(edge.target) ?? new Set<string>()
    if (ports.has(edge.targetHandle)) return `Node ${edge.target} already has a connection on ${edge.targetHandle}`
    ports.add(edge.targetHandle)
    connectedPorts.set(edge.target, ports)
  }

  for (const node of nodes) {
    const nodeType = String(node.data.nodeType ?? node.type)
    const config = (node.data.config as Record<string, unknown>) ?? {}
    const ports = connectedPorts.get(node.id) ?? new Set<string>()
    const validTargetPorts = new Set(getInputPorts(nodeType, config).map((port) => port.id))

    for (const port of ports) {
      if (!validTargetPorts.has(port)) return `Node ${node.id} has invalid input port ${port}`
    }

    switch (nodeType) {
      case 'operator':
        if (!ports.has('left') || !ports.has('right')) return `Operator node ${node.id} must have left and right inputs`
        break
      case 'function':
        if (config.fn === 'min' || config.fn === 'max') {
          if (!ports.has('left') || !ports.has('right')) return `Function node ${node.id} must have left and right inputs`
        } else if (!ports.has('in')) {
          return `Function node ${node.id} must have an in input`
        }
        break
      case 'subFormula':
        if (!ports.has('in')) return `Sub-formula node ${node.id} must have an in input`
        break
      case 'tableLookup':
        if (!ports.has('key')) return `Table lookup node ${node.id} must have a key input`
        break
      case 'conditional':
        for (const port of ['condition', 'conditionRight', 'thenValue', 'elseValue']) {
          if (!ports.has(port)) return `Conditional node ${node.id} must have ${port} input`
        }
        break
      case 'aggregate':
        if (!ports.has('items')) return `Aggregate node ${node.id} must have an items input`
        break
    }
  }

  const outputNodes = nodes.filter((n) => edges.every((e) => e.source !== n.id))
  if (outputNodes.length === 0) return 'Graph must contain at least one output node'

  return null
}

export default function FormulaEditorPage() {
  const { id } = useParams<{ id: string }>()
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { editorMode, setEditorMode, setCurrentFormula, setCurrentVersion } = useFormulaStore()

  const [nodes, setNodes] = useState<Node[]>([])
  const [edges, setEdges] = useState<Edge[]>([])
  const [selectedNode, setSelectedNode] = useState<Node | null>(null)
  const [textValue, setTextValue] = useState('')
  const [testInputs, setTestInputs] = useState<Record<string, string>>({})
  const [testResult, setTestResult] = useState<Record<string, string> | null>(null)
  const [isTestPanelCollapsed, setIsTestPanelCollapsed] = useState(false)
  const [isSaving, setIsSaving] = useState(false)
  const [saveMessage, setSaveMessage] = useState<string | null>(null)

  const { data: formula } = useQuery({
    queryKey: ['formula', id],
    queryFn: () => api.get<Formula>(`/formulas/${id}`),
    enabled: !!id,
  })

  const { data: versions } = useQuery({
    queryKey: ['versions', id],
    queryFn: () => api.get<{ versions: FormulaVersion[] }>(`/formulas/${id}/versions`).then((r) => r.versions ?? []),
    enabled: !!id,
  })

  const latestVersion = versions?.[0]

  useEffect(() => {
    if (formula) setCurrentFormula(formula)
    if (latestVersion) {
      setCurrentVersion(latestVersion)
      if (latestVersion.graph) {
        const { nodes: n, edges: e } = apiToReactFlow(latestVersion.graph)
        setNodes(n)
        setEdges(e)
      }
    }
  }, [formula, latestVersion, setCurrentFormula, setCurrentVersion])

  const handleNodeDataChange = useCallback(
    (nodeId: string, data: Record<string, unknown>) => {
      setNodes((prev) =>
        prev.map((n) => {
          if (n.id !== nodeId) return n
          const nodeType = String(data.nodeType ?? n.data.nodeType ?? n.type) as NodeType
          const config = (data.config as Record<string, unknown>) ?? {}
          return { ...n, data: createNodeData(nodeType, config) }
        })
      )
    },
    []
  )

  const handleSave = async () => {
    if (!id) return
    setIsSaving(true)
    setSaveMessage(null)
    try {
      const validationError = validateGraph(nodes, edges)
      if (validationError) {
        setSaveMessage(validationError)
        return
      }

      const outputNodes = nodes
        .filter((n) => edges.every((e) => e.source !== n.id))
        .map((n) => n.id)
      const graph = reactFlowToApi(nodes, edges, outputNodes)
      await api.post(`/formulas/${id}/versions`, {
        graph,
        changeNote: 'Updated via editor',
      })
      await queryClient.invalidateQueries({ queryKey: ['versions', id] })
      setSaveMessage(t('editor.saved'))
      setTimeout(() => setSaveMessage(null), 3000)
    } catch (err) {
      setSaveMessage((err as Error).message)
    } finally {
      setIsSaving(false)
    }
  }

  const handleTest = async () => {
    if (!id) return
    try {
      const res = await api.post<{ result: Record<string, string> }>('/calculate', {
        formulaId: id,
        inputs: testInputs,
      })
      setTestResult(res.result)
    } catch (err) {
      setTestResult({ error: (err as Error).message })
    }
  }

  return (
    <div className="min-h-screen flex flex-col overflow-x-scroll overflow-y-scroll bg-white">
      {/* Header */}
      <div className="shrink-0 flex flex-wrap items-center justify-between gap-2 border-b border-gray-200 bg-white px-4 py-2">
        <div className="flex min-w-0 flex-wrap items-center gap-3">
          <Link to="/" className="text-gray-400 hover:text-gray-600">&larr;</Link>
          <h1 className="min-w-0 text-lg font-semibold">{formula?.name ?? '...'}</h1>
          <span className="text-xs px-2 py-0.5 bg-blue-100 text-blue-700 rounded">
            {formula?.domain ? t(`domain.${formula.domain}`) : ''}
          </span>
          {latestVersion && (
            <span className="text-xs text-gray-400">v{latestVersion.version}</span>
          )}
        </div>
        <div className="flex flex-wrap items-center justify-end gap-2">
          <div className="flex bg-gray-100 rounded-md p-0.5">
            <button
              onClick={() => setEditorMode('visual')}
              className={`px-3 py-1 text-xs rounded ${editorMode === 'visual' ? 'bg-white shadow-sm' : 'text-gray-500'}`}
            >
              {t('editor.visual')}
            </button>
            <button
              onClick={() => setEditorMode('text')}
              className={`px-3 py-1 text-xs rounded ${editorMode === 'text' ? 'bg-white shadow-sm' : 'text-gray-500'}`}
            >
              {t('editor.text')}
            </button>
          </div>
          <Link
            to={`/formulas/${id}/versions`}
            className="text-xs text-blue-600 hover:underline"
          >
            {t('version.versions')}
          </Link>
          {saveMessage && (
            <span className="text-xs text-green-600">{saveMessage}</span>
          )}
          <button
            onClick={handleSave}
            disabled={isSaving}
            className="bg-blue-600 text-white px-4 py-1.5 rounded text-sm hover:bg-blue-700 disabled:opacity-50"
          >
            {isSaving ? t('common.loading') : t('editor.save')}
          </button>
        </div>
      </div>

      {/* Editor Area */}
      <div className={`min-h-[520px] flex-1 overflow-x-scroll overflow-y-scroll ${isTestPanelCollapsed ? 'pb-20' : 'pb-44'}`}>
        {editorMode === 'visual' ? (
          <div className="flex min-h-[520px] min-w-[1180px]">
            <NodePalette />
            <FormulaCanvas
              nodes={nodes}
              edges={edges}
              onNodesChange={setNodes}
              onEdgesChange={setEdges}
              onNodeSelect={setSelectedNode}
            />
            <NodePropertiesPanel node={selectedNode} onChange={handleNodeDataChange} />
          </div>
        ) : (
          <div className="min-h-[520px] min-w-[820px]">
            <TextEditor value={textValue} onChange={setTextValue} />
          </div>
        )}
      </div>

      {/* Test Panel */}
      <div className="pointer-events-none fixed inset-x-0 bottom-0 z-40 px-3 pb-3 sm:px-4">
        <div className="pointer-events-auto mx-auto max-w-[1440px] rounded-t-xl border border-gray-200 bg-gray-50/95 shadow-[0_-12px_28px_rgba(15,23,42,0.16)] backdrop-blur supports-[backdrop-filter]:bg-gray-50/90">
          <div className="flex items-center justify-between gap-3 border-b border-gray-200 px-4 py-2">
            <div className="flex items-center gap-2">
              <span className="text-sm font-medium text-gray-700">{t('editor.test')}</span>
              <span className="text-xs text-gray-400">
                {isTestPanelCollapsed ? 'Collapsed' : 'Expanded'}
              </span>
            </div>
            <button
              onClick={() => setIsTestPanelCollapsed((prev) => !prev)}
              className="rounded border border-gray-300 bg-white px-3 py-1 text-xs text-gray-600 hover:bg-gray-100"
            >
              {isTestPanelCollapsed ? 'Expand' : 'Collapse'}
            </button>
          </div>

          {!isTestPanelCollapsed && (
            <div className="max-h-[45vh] overflow-auto px-4 py-3">
              <div className="flex flex-wrap items-center gap-3">
                <input
                  className="min-w-[320px] flex-1 text-sm border border-gray-300 rounded px-2 py-1"
                  placeholder={`${t('calc.inputs')} (JSON): {"age": "35", "sumAssured": "1000000"}`}
                  onChange={(e) => {
                    try { setTestInputs(JSON.parse(e.target.value)) } catch { /* ignore */ }
                  }}
                />
                <button
                  onClick={handleTest}
                  className="bg-green-600 text-white px-4 py-1.5 rounded text-sm hover:bg-green-700"
                >
                  {t('calc.calculate')}
                </button>
              </div>
              {testResult && (
                <div className="mt-3 overflow-auto rounded border bg-white p-2 text-xs font-mono text-gray-700">
                  {JSON.stringify(testResult)}
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
