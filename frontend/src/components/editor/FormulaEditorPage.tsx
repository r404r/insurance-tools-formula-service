import { useEffect, useState, useCallback, useMemo, useRef } from 'react'
import { useParams, Link, useSearchParams, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import type { Node, Edge } from '@xyflow/react'
import { useFormulaStore } from '../../store/formulaStore'
import { api } from '../../api/client'
import { listCategories } from '../../api/categories'
import { useAuthStore } from '../../store/authStore'
import { apiToReactFlow, reactFlowToApi } from '../../utils/graphSerializer'
import { reactFlowToText } from '../../utils/graphText'
import { parseFormula } from '../../api/parse'
import FormulaCanvas from './FormulaCanvas'
import TextEditor from './TextEditor'
import NodePalette from './NodePalette'
import NodePropertiesPanel from './NodePropertiesPanel'
import type { Formula, FormulaVersion, InsuranceDomain, NodeType } from '../../types/formula'
import { createNodeData } from './nodePresentation'
import { useAutoLayout } from './hooks/useAutoLayout'
import { validateGraph, type ValidationIssue } from '../../utils/graphValidation'
export type { ValidationIssue } from '../../utils/graphValidation'

export default function FormulaEditorPage() {
  const { id } = useParams<{ id: string }>()
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const user = useAuthStore((state) => state.user)
  const editorMode = useFormulaStore((state) => state.editorMode)
  const setEditorMode = useFormulaStore((state) => state.setEditorMode)
  // Allow ?mode=text URL param to set the initial mode (useful for automated tests).
  // Clicking a mode button clears the param so the Zustand store takes over.
  const modeParam = searchParams.get('mode')
  const effectiveMode: 'visual' | 'text' =
    modeParam === 'text' || modeParam === 'visual' ? modeParam : editorMode

  const setCurrentFormula = useFormulaStore((state) => state.setCurrentFormula)
  const setCurrentVersion = useFormulaStore((state) => state.setCurrentVersion)

  const [nodes, setNodes] = useState<Node[]>([])

  const handleSetEditorMode = useCallback(
    (mode: 'visual' | 'text') => {
      if (mode === 'text') {
        // Warn if any loop node has non-default settings that text mode can't represent
        const lossyLoop = nodes.some((n) => {
          if ((n.data.nodeType as string) !== 'loop') return false
          const cfg = (n.data.config as Record<string, unknown>) ?? {}
          return cfg.inclusiveEnd === false || cfg.maxIterations != null || cfg.version != null
        })
        if (lossyLoop) {
          setSaveMessage(t('editor.loopTextLossy'))
        }
      }
      setEditorMode(mode)
      if (modeParam) {
        // Remove the ?mode param so subsequent mode switches use the Zustand store
        navigate(`/formulas/${id}`, { replace: true })
      }
    },
    [id, modeParam, navigate, nodes, setEditorMode, t]
  )
  const [edges, setEdges] = useState<Edge[]>([])
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null)
  const [textValue, setTextValue] = useState('')
  const [testInputs, setTestInputs] = useState<Record<string, string>>({})
  const [testResult, setTestResult] = useState<Record<string, string> | null>(null)
  const [isTestPanelCollapsed, setIsTestPanelCollapsed] = useState(false)
  const [validationIssues, setValidationIssues] = useState<ValidationIssue[]>([])
  const validationState = useMemo(() => ({
    invalidNodeIds: new Set(
      validationIssues.filter((i) => i.severity === 'error').flatMap((i) => i.nodeIds)
    ),
    warnNodeIds: new Set(
      validationIssues.filter((i) => i.severity === 'warning').flatMap((i) => i.nodeIds)
    ),
  }), [validationIssues])
  const saveMessageTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const [isParsing, setIsParsing] = useState(false)
  const [isSaving, setIsSaving] = useState(false)
  const [isRenaming, setIsRenaming] = useState(false)
  const [isUpdatingCategory, setIsUpdatingCategory] = useState(false)
  const [isEditingName, setIsEditingName] = useState(false)
  const [nameDraft, setNameDraft] = useState('')
  const [isEditingDesc, setIsEditingDesc] = useState(false)
  const [descDraft, setDescDraft] = useState('')
  const [isSavingDesc, setIsSavingDesc] = useState(false)
  const [saveMessage, setSaveMessage] = useState<string | null>(null)
  const [activeVersionNumber, setActiveVersionNumber] = useState<number | null>(null)
  const autoLayout = useAutoLayout()
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

      let changed = false
      const result = inputNodes.map((node) => {
        const nodeType = String(node.data.nodeType ?? node.type)
        if (nodeType !== 'subFormula' && nodeType !== 'loop') {
          return node
        }

        const config = (node.data.config as Record<string, unknown>) ?? {}
        const formulaId = String(config.formulaId ?? '')
        const matchedFormula = allFormulas.find((item) => item.id === formulaId)
        const formulaName = matchedFormula?.name ?? String(config.formulaName ?? '').trim()

        if (!formulaName || formulaName === String(config.formulaName ?? '')) {
          return node
        }

        changed = true
        return {
          ...node,
          data: createNodeData(nodeType as NodeType, {
            ...config,
            formulaName,
          }, String(node.data.description ?? '')),
        }
      })
      return changed ? result : inputNodes
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
    setDescDraft(formula?.description ?? '')
  }, [formula?.description])

  // Clear stale validation highlights when graph structure changes.
  // Uses functional updater: if already empty, returns same reference → React bails out and skips
  // the context value update, preventing unnecessary FormulaNode re-renders on every drag event.
  useEffect(() => {
    setValidationIssues((prev) => (prev.length === 0 ? prev : []))
  }, [nodes, edges])

  useEffect(() => {
    return () => {
      if (saveMessageTimeoutRef.current !== null) clearTimeout(saveMessageTimeoutRef.current)
    }
  }, [])

  useEffect(() => {
    if (effectiveMode !== 'text') {
      return
    }

    try {
      setTextValue(reactFlowToText(nodes, edges))
    } catch (err) {
      setTextValue(`// ${(err as Error).message}`)
    }
  }, [effectiveMode, nodes, edges])

  const handleNodeDataChange = useCallback(
    (nodeId: string, data: Record<string, unknown>) => {
      setNodes((prev) =>
        prev.map((n) => {
          if (n.id !== nodeId) return n
          const nodeType = String(data.nodeType ?? n.data.nodeType ?? n.type) as NodeType
          const config = (data.config as Record<string, unknown>) ?? {}
          return { ...n, data: createNodeData(nodeType, config, String(data.description ?? n.data.description ?? '')) }
        })
      )
    },
    []
  )

  const handleApplyText = useCallback(async (text: string) => {
    setTextValue(text)
    setIsParsing(true)
    setSaveMessage(null)
    try {
      const { graph } = await parseFormula(text)
      const { nodes: newNodes, edges: newEdges } = apiToReactFlow(graph)
      const hydratedNodes = enrichSubFormulaNodes(newNodes)
      const layoutNodes = autoLayout(hydratedNodes, newEdges)
      setNodes(layoutNodes)
      setEdges(newEdges)
      setSelectedNodeId(null)
      setSaveMessage(t('editor.textApplied'))
      setTimeout(() => setSaveMessage(null), 3000)
    } catch (err) {
      setSaveMessage((err as Error).message)
    } finally {
      setIsParsing(false)
    }
  }, [autoLayout, enrichSubFormulaNodes, t])

  const handleSave = async () => {
    if (!id) return
    // Cancel any pending "clear highlights" timeout from a prior save
    if (saveMessageTimeoutRef.current !== null) {
      clearTimeout(saveMessageTimeoutRef.current)
      saveMessageTimeoutRef.current = null
    }
    setIsSaving(true)
    setSaveMessage(null)
    setValidationIssues([])
    try {
      // Step 1: Frontend structural validation (instant feedback)
      const frontendIssues = validateGraph(nodes, edges)
      const frontendErrors = frontendIssues.filter((i) => i.severity === 'error')
      if (frontendErrors.length > 0) {
        setValidationIssues(frontendIssues)
        setSaveMessage(frontendErrors.map((e) => e.message).join(' · '))
        return
      }

      const saveSourceIds = new Set(edges.map((e) => e.source))
      const outputNodes = nodes
        .filter((n) => !saveSourceIds.has(n.id))
        .map((n) => n.id)
      const graph = reactFlowToApi(nodes, edges, outputNodes)

      // Step 2: Backend deep validation (cycle detection, engine-level checks)
      try {
        const validationResult = await api.post<{ valid: boolean; errors: { nodeId: string; message: string }[] }>(
          '/calculate/validate',
          graph
        )
        const backendErrors = validationResult.errors ?? []
        if (!validationResult.valid && backendErrors.length > 0) {
          const backendIssues: ValidationIssue[] = backendErrors.map((e) => ({
            message: e.message,
            nodeIds: e.nodeId ? [e.nodeId] : [],
            severity: 'error' as const,
          }))
          setValidationIssues([...frontendIssues, ...backendIssues])
          setSaveMessage(backendErrors.map((e) => e.message).join(' · '))
          return
        }
      } catch {
        // Backend validation is best-effort; don't block save on network error
      }

      // Carry through any frontend warnings even on success
      if (frontendIssues.length > 0) setValidationIssues(frontendIssues)

      // Step 3: Save
      const savedVersion = await api.post<FormulaVersion>(`/formulas/${id}/versions`, {
        graph,
        changeNote: 'Updated via editor',
      })
      setCurrentVersion(savedVersion)
      setActiveVersionNumber(savedVersion.version)
      await queryClient.invalidateQueries({ queryKey: ['versions', id] })
      setSaveMessage(t('editor.saved'))
      saveMessageTimeoutRef.current = setTimeout(() => {
        setSaveMessage(null)
        setValidationIssues([])
        saveMessageTimeoutRef.current = null
      }, 3000)
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

  const handleDescSave = useCallback(async () => {
    if (!id || !formula) return

    const trimmed = descDraft.trim()
    if (trimmed === (formula.description ?? '').trim()) {
      setIsEditingDesc(false)
      return
    }

    setIsSavingDesc(true)
    setSaveMessage(null)
    try {
      const updatedFormula = await api.put<Formula>(`/formulas/${id}`, { description: trimmed })
      setCurrentFormula(updatedFormula)
      setDescDraft(updatedFormula.description ?? '')
      setIsEditingDesc(false)
      setSaveMessage(t('editor.saved'))
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['formula', id] }),
        queryClient.invalidateQueries({ queryKey: ['formulas'] }),
      ])
      setTimeout(() => setSaveMessage(null), 3000)
    } catch (err) {
      setSaveMessage((err as Error).message)
      setDescDraft(formula.description ?? '')
    } finally {
      setIsSavingDesc(false)
    }
  }, [descDraft, formula, id, queryClient, setCurrentFormula, t])

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
      <div className="shrink-0 flex flex-col gap-1 border-b border-gray-200 bg-white px-4 py-2">
        <div className="flex flex-wrap items-center justify-between gap-2">
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
              data-testid="mode-visual"
              onClick={() => handleSetEditorMode('visual')}
              className={`px-3 py-1 text-xs rounded ${effectiveMode === 'visual' ? 'bg-white shadow-sm' : 'text-gray-500'}`}
            >
              {t('editor.visual')}
            </button>
            <button
              data-testid="mode-text"
              onClick={() => handleSetEditorMode('text')}
              className={`px-3 py-1 text-xs rounded ${effectiveMode === 'text' ? 'bg-white shadow-sm' : 'text-gray-500'}`}
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
            <span className={`max-w-[360px] truncate text-xs ${saveMessage === t('editor.saved') ? 'text-green-600' : 'text-red-600'}`} title={saveMessage}>{saveMessage}</span>
          )}
          {validationIssues.some((i) => i.severity === 'warning') && !saveMessage && (
            <span className="text-xs text-amber-600">
              ⚠ {validationIssues.filter((i) => i.severity === 'warning').length} warning(s)
            </span>
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

        {/* Description row */}
        {isEditor ? (
          isEditingDesc ? (
            <input
              autoFocus
              value={descDraft}
              onChange={(e) => setDescDraft(e.target.value)}
              onBlur={() => { void handleDescSave() }}
              onKeyDown={(e) => {
                if (e.key === 'Enter') { e.preventDefault(); void handleDescSave() }
                if (e.key === 'Escape') { setDescDraft(formula?.description ?? ''); setIsEditingDesc(false) }
              }}
              disabled={isSavingDesc}
              placeholder={t('formula.description')}
              className="w-full rounded border border-blue-300 bg-white px-2 py-0.5 text-xs text-slate-600 shadow-sm outline-none ring-2 ring-blue-100"
            />
          ) : (
            <button
              type="button"
              onClick={() => { setDescDraft(formula?.description ?? ''); setIsEditingDesc(true) }}
              className="w-full truncate rounded px-1 text-left text-xs text-slate-500 hover:bg-slate-100"
              title={formula?.description || t('formula.description')}
            >
              {formula?.description || <span className="italic text-slate-400">{t('formula.description')}</span>}
            </button>
          )
        ) : formula?.description ? (
          <p className="truncate px-1 text-xs text-slate-500">{formula.description}</p>
        ) : null}
      </div>

      {/* Editor Area */}
      <div className={`min-h-[520px] flex-[1_0_520px] overflow-x-scroll overflow-y-scroll ${isTestPanelCollapsed ? 'pb-20' : 'pb-44'}`}>
        {effectiveMode === 'visual' ? (
          <div className="flex h-full min-h-[520px] min-w-[1180px]">
            <NodePalette />
            <FormulaCanvas
              nodes={nodes}
              edges={edges}
              onNodesChange={setNodes}
              onEdgesChange={setEdges}
              onNodeSelect={(node) => setSelectedNodeId(node?.id ?? null)}
              validation={validationState}
            />
            <NodePropertiesPanel node={selectedNode} onChange={handleNodeDataChange} currentFormulaId={id ?? null} />
          </div>
        ) : (
          <div className="h-full min-h-[520px] min-w-[820px]">
            <TextEditor value={textValue} onChange={handleApplyText} isParsing={isParsing} />
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
