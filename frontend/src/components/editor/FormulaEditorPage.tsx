import { useEffect, useState, useCallback } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import type { Node, Edge } from '@xyflow/react'
import { useFormulaStore } from '../../store/formulaStore'
import { api } from '../../api/client'
import { listCategories } from '../../api/categories'
import { useAuthStore } from '../../store/authStore'
import { apiToReactFlow, reactFlowToApi } from '../../utils/graphSerializer'
import { reactFlowToText } from '../../utils/graphText'
import FormulaCanvas from './FormulaCanvas'
import TextEditor from './TextEditor'
import NodePalette from './NodePalette'
import NodePropertiesPanel from './NodePropertiesPanel'
import type { Formula, FormulaVersion, InsuranceDomain, NodeType } from '../../types/formula'
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
        if (!String(config.formulaId ?? '').trim()) return `Sub-formula node ${node.id} must reference a formula`
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
  const user = useAuthStore((state) => state.user)
  const editorMode = useFormulaStore((state) => state.editorMode)
  const setEditorMode = useFormulaStore((state) => state.setEditorMode)
  const setCurrentFormula = useFormulaStore((state) => state.setCurrentFormula)
  const setCurrentVersion = useFormulaStore((state) => state.setCurrentVersion)

  const [nodes, setNodes] = useState<Node[]>([])
  const [edges, setEdges] = useState<Edge[]>([])
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null)
  const [textValue, setTextValue] = useState('')
  const [testInputs, setTestInputs] = useState<Record<string, string>>({})
  const [testResult, setTestResult] = useState<Record<string, string> | null>(null)
  const [isTestPanelCollapsed, setIsTestPanelCollapsed] = useState(false)
  const [isSaving, setIsSaving] = useState(false)
  const [isRenaming, setIsRenaming] = useState(false)
  const [isUpdatingCategory, setIsUpdatingCategory] = useState(false)
  const [isEditingName, setIsEditingName] = useState(false)
  const [nameDraft, setNameDraft] = useState('')
  const [saveMessage, setSaveMessage] = useState<string | null>(null)
  const [activeVersionNumber, setActiveVersionNumber] = useState<number | null>(null)
  const selectedNode = selectedNodeId ? nodes.find((node) => node.id === selectedNodeId) ?? null : null
  const isEditor = user?.role === 'editor' || user?.role === 'admin'

  const { data: formula } = useQuery({
    queryKey: ['formula', id],
    queryFn: () => api.get<Formula>(`/formulas/${id}`),
    enabled: !!id,
  })

  const { data: allFormulas = [] } = useQuery({
    queryKey: ['formulas', 'editor-options'],
    queryFn: () =>
      api
        .get<{ formulas: Formula[]; total: number }>('/formulas')
        .then((response) => response.formulas ?? []),
    enabled: !!id,
  })

  const { data: versions } = useQuery({
    queryKey: ['versions', id],
    queryFn: () => api.get<{ versions: FormulaVersion[] }>(`/formulas/${id}/versions`).then((r) => r.versions ?? []),
    enabled: !!id,
  })

  const { data: categories = [] } = useQuery({
    queryKey: ['categories'],
    queryFn: () => listCategories().then((response) => response.categories ?? []),
  })

  const latestVersion = versions?.[0]

  const enrichSubFormulaNodes = useCallback(
    (inputNodes: Node[]) => {
      if (allFormulas.length === 0) {
        return inputNodes
      }

      return inputNodes.map((node) => {
        const nodeType = String(node.data.nodeType ?? node.type)
        if (nodeType !== 'subFormula') {
          return node
        }

        const config = (node.data.config as Record<string, unknown>) ?? {}
        const formulaId = String(config.formulaId ?? '')
        const matchedFormula = allFormulas.find((item) => item.id === formulaId)
        const formulaName = matchedFormula?.name ?? String(config.formulaName ?? '').trim()

        if (!formulaName || formulaName === String(config.formulaName ?? '')) {
          return node
        }

        return {
          ...node,
          data: createNodeData('subFormula', {
            ...config,
            formulaName,
          }),
        }
      })
    },
    [allFormulas]
  )

  useEffect(() => {
    if (!formula) {
      return
    }

    setCurrentFormula(formula)
  }, [formula?.id, formula?.updatedAt, formula, setCurrentFormula])

  useEffect(() => {
    if (!latestVersion) {
      return
    }

    setCurrentVersion(latestVersion)
    setActiveVersionNumber(latestVersion.version)
  }, [latestVersion?.id, latestVersion?.version, latestVersion, setCurrentVersion])

  useEffect(() => {
    if (!latestVersion?.graph) {
      return
    }

    const { nodes: n, edges: e } = apiToReactFlow(latestVersion.graph)
    const hydratedNodes = enrichSubFormulaNodes(n)
    setNodes(hydratedNodes)
    setEdges(e)
    try {
      setTextValue(reactFlowToText(hydratedNodes, e))
    } catch (err) {
      setTextValue(`// ${(err as Error).message}`)
    }
    setSelectedNodeId(null)
  }, [enrichSubFormulaNodes, latestVersion?.id, latestVersion?.graph])

  useEffect(() => {
    if (allFormulas.length === 0) {
      return
    }

    setNodes((prev) => enrichSubFormulaNodes(prev))
  }, [allFormulas, enrichSubFormulaNodes])

  useEffect(() => {
    setNameDraft(formula?.name ?? '')
  }, [formula?.name])

  useEffect(() => {
    if (editorMode !== 'text') {
      return
    }

    try {
      setTextValue(reactFlowToText(nodes, edges))
    } catch (err) {
      setTextValue(`// ${(err as Error).message}`)
    }
  }, [editorMode, nodes, edges])

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
      const savedVersion = await api.post<FormulaVersion>(`/formulas/${id}/versions`, {
        graph,
        changeNote: 'Updated via editor',
      })
      setCurrentVersion(savedVersion)
      setActiveVersionNumber(savedVersion.version)
      await queryClient.invalidateQueries({ queryKey: ['versions', id] })
      setSaveMessage(t('editor.saved'))
      setTimeout(() => setSaveMessage(null), 3000)
    } catch (err) {
      setSaveMessage((err as Error).message)
    } finally {
      setIsSaving(false)
    }
  }

  const handleNameSave = useCallback(async () => {
    if (!id || !formula) return

    const trimmedName = nameDraft.trim()
    if (!trimmedName) {
      setNameDraft(formula.name)
      setIsEditingName(false)
      setSaveMessage('Formula name is required')
      return
    }

    if (trimmedName === formula.name) {
      setIsEditingName(false)
      return
    }

    setIsRenaming(true)
    setSaveMessage(null)
    try {
      const updatedFormula = await api.put<Formula>(`/formulas/${id}`, {
        name: trimmedName,
      })
      setCurrentFormula(updatedFormula)
      setNameDraft(updatedFormula.name)
      setIsEditingName(false)
      setSaveMessage(t('editor.saved'))
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['formula', id] }),
        queryClient.invalidateQueries({ queryKey: ['formulas'] }),
      ])
      setTimeout(() => setSaveMessage(null), 3000)
    } catch (err) {
      setSaveMessage((err as Error).message)
      setNameDraft(formula.name)
    } finally {
      setIsRenaming(false)
    }
  }, [formula, id, nameDraft, queryClient, setCurrentFormula, t])

  const handleCategoryChange = useCallback(
    async (nextDomain: InsuranceDomain) => {
      if (!id || !formula || !nextDomain || nextDomain === formula.domain) {
        return
      }

      setIsUpdatingCategory(true)
      setSaveMessage(null)
      try {
        const updatedFormula = await api.put<Formula>(`/formulas/${id}`, {
          domain: nextDomain,
        })
        setCurrentFormula(updatedFormula)
        await Promise.all([
          queryClient.invalidateQueries({ queryKey: ['formula', id] }),
          queryClient.invalidateQueries({ queryKey: ['formulas'] }),
          queryClient.invalidateQueries({ queryKey: ['categories'] }),
        ])
        setSaveMessage(t('editor.saved'))
        setTimeout(() => setSaveMessage(null), 3000)
      } catch (err) {
        setSaveMessage((err as Error).message)
      } finally {
        setIsUpdatingCategory(false)
      }
    },
    [formula, id, queryClient, setCurrentFormula, t]
  )

  const handleTest = async () => {
    if (!id) return
    try {
      const res = await api.post<{ result: Record<string, string> }>('/calculate', {
        formulaId: id,
        version: activeVersionNumber ?? latestVersion?.version,
        inputs: testInputs,
      })
      setTestResult(res.result)
    } catch (err) {
      setTestResult({ error: (err as Error).message })
    }
  }

  const currentCategory = categories.find((category) => category.slug === formula?.domain)
  const currentCategoryColor = currentCategory?.color ?? '#2563eb'
  const currentCategoryLabel = currentCategory?.name ?? formula?.domain ?? ''

  return (
    <div className="min-h-screen flex flex-col overflow-x-scroll overflow-y-scroll bg-white">
      {/* Header */}
      <div className="shrink-0 flex flex-wrap items-center justify-between gap-2 border-b border-gray-200 bg-white px-4 py-2">
        <div className="flex min-w-0 flex-wrap items-center gap-3">
          <Link to="/" className="text-gray-400 hover:text-gray-600">&larr;</Link>
          {isEditingName ? (
            <input
              autoFocus
              value={nameDraft}
              onChange={(e) => setNameDraft(e.target.value)}
              onBlur={() => {
                void handleNameSave()
              }}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault()
                  void handleNameSave()
                }
                if (e.key === 'Escape') {
                  setNameDraft(formula?.name ?? '')
                  setIsEditingName(false)
                }
              }}
              disabled={isRenaming}
              className="min-w-[220px] max-w-[420px] rounded border border-blue-300 bg-white px-2 py-1 text-lg font-semibold text-slate-900 shadow-sm outline-none ring-2 ring-blue-100"
            />
          ) : (
            <button
              type="button"
              onClick={() => {
                setNameDraft(formula?.name ?? '')
                setIsEditingName(true)
              }}
              className="min-w-0 truncate rounded px-1 text-left text-lg font-semibold text-slate-900 hover:bg-slate-100"
              title={formula?.name ?? ''}
            >
              {formula?.name ?? '...'}
            </button>
          )}
          {formula?.id && (
            <span className="max-w-[360px] truncate rounded bg-slate-100 px-2 py-0.5 font-mono text-[11px] text-slate-600">
              ID: {formula.id}
            </span>
          )}
          {isEditor ? (
            <select
              value={formula?.domain ?? ''}
              onChange={(e) => {
                void handleCategoryChange(e.target.value)
              }}
              disabled={isUpdatingCategory || categories.length === 0}
              className="rounded border border-slate-200 bg-white px-2 py-1 text-xs text-slate-700"
            >
              {formula?.domain && !currentCategory && (
                <option value={formula.domain}>{currentCategoryLabel}</option>
              )}
              {categories.map((category) => (
                <option key={category.id} value={category.slug}>
                  {category.name}
                </option>
              ))}
            </select>
          ) : (
            <span
              className="rounded px-2 py-0.5 text-xs"
              style={{
                color: currentCategoryColor,
                backgroundColor: `${currentCategoryColor}18`,
              }}
            >
              {currentCategoryLabel}
            </span>
          )}
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
            <span className={`text-xs ${saveMessage === t('editor.saved') ? 'text-green-600' : 'text-red-600'}`}>{saveMessage}</span>
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
      <div className={`min-h-[520px] flex-[1_0_520px] overflow-x-scroll overflow-y-scroll ${isTestPanelCollapsed ? 'pb-20' : 'pb-44'}`}>
        {editorMode === 'visual' ? (
          <div className="flex h-full min-h-[520px] min-w-[1180px]">
            <NodePalette />
            <FormulaCanvas
              nodes={nodes}
              edges={edges}
              onNodesChange={setNodes}
              onEdgesChange={setEdges}
              onNodeSelect={(node) => setSelectedNodeId(node?.id ?? null)}
            />
            <NodePropertiesPanel node={selectedNode} onChange={handleNodeDataChange} currentFormulaId={id ?? null} />
          </div>
        ) : (
          <div className="h-full min-h-[520px] min-w-[820px]">
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
