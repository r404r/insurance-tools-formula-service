import { useEffect, useState, useCallback } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import type { Node, Edge } from '@xyflow/react'
import { useFormulaStore } from '../../store/formulaStore'
import { api } from '../../api/client'
import { apiToReactFlow, reactFlowToApi } from '../../utils/graphSerializer'
import FormulaCanvas from './FormulaCanvas'
import TextEditor from './TextEditor'
import NodePalette from './NodePalette'
import NodePropertiesPanel from './NodePropertiesPanel'
import type { Formula, FormulaVersion } from '../../types/formula'

export default function FormulaEditorPage() {
  const { id } = useParams<{ id: string }>()
  const { t } = useTranslation()
  const { editorMode, setEditorMode, setCurrentFormula, setCurrentVersion } = useFormulaStore()

  const [nodes, setNodes] = useState<Node[]>([])
  const [edges, setEdges] = useState<Edge[]>([])
  const [selectedNode, setSelectedNode] = useState<Node | null>(null)
  const [textValue, setTextValue] = useState('')
  const [testInputs, setTestInputs] = useState<Record<string, string>>({})
  const [testResult, setTestResult] = useState<Record<string, string> | null>(null)
  const [isSaving, setIsSaving] = useState(false)

  const { data: formula } = useQuery({
    queryKey: ['formula', id],
    queryFn: () => api.get<Formula>(`/formulas/${id}`),
    enabled: !!id,
  })

  const { data: versions } = useQuery({
    queryKey: ['versions', id],
    queryFn: () => api.get<{ versions: FormulaVersion[] }>(`/formulas/${id}/versions`).then((r) => r.versions),
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
      setNodes((prev) => prev.map((n) => (n.id === nodeId ? { ...n, data } : n)))
    },
    []
  )

  const handleSave = async () => {
    if (!id) return
    setIsSaving(true)
    try {
      const graph = reactFlowToApi(nodes, edges, nodes.filter((n) => edges.every((e) => e.source !== n.id)).map((n) => n.id))
      await api.post(`/formulas/${id}/versions`, {
        graph,
        changeNote: 'Updated via editor',
      })
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
    <div className="h-screen flex flex-col">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-2 border-b border-gray-200 bg-white">
        <div className="flex items-center gap-3">
          <Link to="/" className="text-gray-400 hover:text-gray-600">&larr;</Link>
          <h1 className="text-lg font-semibold">{formula?.name ?? '...'}</h1>
          <span className="text-xs px-2 py-0.5 bg-blue-100 text-blue-700 rounded">
            {formula?.domain ? t(`domain.${formula.domain}`) : ''}
          </span>
          {latestVersion && (
            <span className="text-xs text-gray-400">v{latestVersion.version}</span>
          )}
        </div>
        <div className="flex items-center gap-2">
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
      <div className="flex-1 flex overflow-hidden">
        {editorMode === 'visual' ? (
          <>
            <NodePalette />
            <FormulaCanvas
              nodes={nodes}
              edges={edges}
              onNodesChange={setNodes}
              onEdgesChange={setEdges}
              onNodeSelect={setSelectedNode}
            />
            <NodePropertiesPanel node={selectedNode} onChange={handleNodeDataChange} />
          </>
        ) : (
          <TextEditor value={textValue} onChange={setTextValue} />
        )}
      </div>

      {/* Test Panel */}
      <div className="border-t border-gray-200 bg-gray-50 p-3">
        <div className="flex items-center gap-3">
          <span className="text-sm font-medium text-gray-600">{t('editor.test')}:</span>
          <input
            className="flex-1 text-sm border border-gray-300 rounded px-2 py-1"
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
          {testResult && (
            <span className="text-xs font-mono text-gray-700 bg-white border rounded px-2 py-1 max-w-md truncate">
              {JSON.stringify(testResult)}
            </span>
          )}
        </div>
      </div>
    </div>
  )
}
