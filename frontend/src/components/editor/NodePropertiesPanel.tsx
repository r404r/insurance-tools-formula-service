import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import type { Node } from '@xyflow/react'
import { api } from '../../api/client'
import { listTables } from '../../api/tables'
import type { Formula, FormulaVersion, LookupTable } from '../../types/formula'
import { validateNodeIdFormat } from '../../utils/nodeIdValidation'

interface Props {
  node: Node | null
  onChange: (id: string, data: Record<string, unknown>) => void
  onIdChange?: (oldId: string, newId: string) => void
  existingNodeIds?: Set<string>
  currentFormulaId?: string | null
}

export default function NodePropertiesPanel({
  node,
  onChange,
  onIdChange,
  existingNodeIds,
  currentFormulaId,
}: Props) {
  const { t } = useTranslation()
  const config = ((node?.data.config as Record<string, unknown> | undefined) ?? {})
  const selectedFormulaId = String(config.formulaId ?? '')

  // ── Editable node ID ──
  // Draft lives in local state so we can validate on every keystroke without
  // committing intermediate values into the graph. Commit on blur or Enter.
  const [idDraft, setIdDraft] = useState(node?.id ?? '')
  const [idError, setIdError] = useState<string | null>(null)
  // Reset draft whenever the selected node changes (or its id is renamed).
  useEffect(() => {
    setIdDraft(node?.id ?? '')
    setIdError(null)
  }, [node?.id])

  const validateIdDraft = (draft: string): string | null => {
    if (!node) return null
    if (draft === node.id) return null
    const formatError = validateNodeIdFormat(draft)
    if (formatError === 'empty') return t('editor.idEmpty')
    if (formatError === 'invalid') return t('editor.idInvalid')
    if (existingNodeIds && existingNodeIds.has(draft)) return t('editor.idConflict')
    return null
  }

  const handleIdInputChange = (value: string) => {
    setIdDraft(value)
    setIdError(validateIdDraft(value))
  }

  const commitIdChange = () => {
    if (!node) return
    // Validate the raw input — do NOT trim. A space or any character that
    // does not match the regex must surface as an error instead of being
    // silently normalized, otherwise the advertised format contract breaks.
    if (idDraft === node.id) {
      setIdError(null)
      return
    }
    const error = validateIdDraft(idDraft)
    if (error) {
      // Keep the draft visible with the error message so the user can fix it,
      // but do not propagate the rename.
      setIdError(error)
      return
    }
    onIdChange?.(node.id, idDraft)
    setIdError(null)
  }

  const handleIdKeyDown = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Enter') {
      event.preventDefault()
      commitIdChange()
      return
    }
    if (event.key === 'Escape') {
      event.preventDefault()
      setIdDraft(node?.id ?? '')
      setIdError(null)
    }
  }

  const { data: formulas = [] } = useQuery({
    queryKey: ['formulas', 'subformula-options'],
    queryFn: () =>
      api
        .get<{ formulas: Formula[]; total: number }>('/formulas')
        .then((response) => response.formulas ?? []),
  })

  const { data: formulaVersions = [] } = useQuery({
    queryKey: ['versions', selectedFormulaId],
    queryFn: () =>
      api
        .get<{ versions: FormulaVersion[] }>(`/formulas/${selectedFormulaId}/versions`)
        .then((response) => response.versions ?? []),
    enabled: !!selectedFormulaId,
  })

  const { data: tables = [] } = useQuery<LookupTable[]>({
    queryKey: ['tables'],
    queryFn: () => listTables().then((r) => r.tables ?? []),
  })

  if (!node) {
    return (
      <div className="w-64 border-l border-gray-200 bg-gray-50 p-4 text-sm text-gray-400">
        {t('editor.properties')}: {t('common.noData')}
      </div>
    )
  }

  const nodeType = (node.data.nodeType as string) ?? node.type

  const updateConfig = (key: string, value: unknown) => {
    onChange(node.id, { ...node.data, config: { ...config, [key]: value } })
  }

  const updateFunctionArg = (key: string, value: string) => {
    const args = (config.args as Record<string, string> | undefined) ?? {}
    onChange(node.id, {
      ...node.data,
      config: {
        ...config,
        args: {
          ...args,
          [key]: value,
        },
      },
    })
  }

  return (
    <div className="w-64 border-l border-gray-200 bg-gray-50 p-4 overflow-y-auto">
      <h3 className="text-sm font-semibold text-gray-600 mb-3">{t('editor.properties')}</h3>
      <div className="space-y-3">
        <div>
          <label className="block text-xs text-gray-500 mb-1">ID</label>
          <input
            className={`w-full text-xs rounded px-2 py-1 border ${
              idError
                ? 'border-red-400 bg-red-50 focus:border-red-500 focus:outline-none focus:ring-1 focus:ring-red-300'
                : 'border-gray-300 bg-white focus:border-indigo-500 focus:outline-none focus:ring-1 focus:ring-indigo-200'
            }`}
            value={idDraft}
            onChange={(e) => handleIdInputChange(e.target.value)}
            onBlur={commitIdChange}
            onKeyDown={handleIdKeyDown}
            disabled={!onIdChange}
            spellCheck={false}
            autoComplete="off"
          />
          {idError && (
            <p className="mt-1 text-[11px] text-red-600">{idError}</p>
          )}
        </div>
        <div>
          <label className="block text-xs text-gray-500 mb-1">Type</label>
          <input className="w-full text-xs bg-gray-100 rounded px-2 py-1" value={nodeType} disabled />
        </div>
        <div>
          <label className="block text-xs text-gray-500 mb-1">{t('formula.description')}</label>
          <textarea
            className="w-full text-xs border border-gray-300 rounded px-2 py-1 resize-none"
            rows={2}
            value={String(node.data.description ?? '')}
            onChange={(e) => onChange(node.id, { ...node.data, description: e.target.value })}
            placeholder={t('formula.description')}
          />
        </div>

        {nodeType === 'variable' && (
          <>
            <div>
              <label className="block text-xs text-gray-500 mb-1">{t('formula.name')}</label>
              <input
                className="w-full text-xs border border-gray-300 rounded px-2 py-1"
                value={(config.name as string) ?? ''}
                onChange={(e) => updateConfig('name', e.target.value)}
              />
            </div>
            <div>
              <label className="block text-xs text-gray-500 mb-1">Data Type</label>
              <select
                className="w-full text-xs border border-gray-300 rounded px-2 py-1"
                value={(config.dataType as string) ?? 'decimal'}
                onChange={(e) => updateConfig('dataType', e.target.value)}
              >
                <option value="integer">Integer</option>
                <option value="decimal">Decimal</option>
                <option value="string">String</option>
                <option value="boolean">Boolean</option>
              </select>
            </div>
          </>
        )}

        {nodeType === 'constant' && (
          <div>
            <label className="block text-xs text-gray-500 mb-1">Value</label>
            <input
              className="w-full text-xs border border-gray-300 rounded px-2 py-1"
              value={(config.value as string) ?? ''}
              onChange={(e) => updateConfig('value', e.target.value)}
            />
          </div>
        )}

        {nodeType === 'operator' && (
          <div>
            <label className="block text-xs text-gray-500 mb-1">Operator</label>
            <select
              className="w-full text-xs border border-gray-300 rounded px-2 py-1"
              value={(config.op as string) ?? 'add'}
              onChange={(e) => updateConfig('op', e.target.value)}
            >
              <option value="add">+ Add</option>
              <option value="subtract">- Subtract</option>
              <option value="multiply">* Multiply</option>
              <option value="divide">/ Divide</option>
              <option value="power">^ Power</option>
              <option value="modulo">% Modulo</option>
            </select>
          </div>
        )}

        {nodeType === 'function' && (
          <>
            <div>
              <label className="block text-xs text-gray-500 mb-1">Function</label>
              <select
                className="w-full text-xs border border-gray-300 rounded px-2 py-1"
                value={(config.fn as string) ?? 'round'}
                onChange={(e) => updateConfig('fn', e.target.value)}
              >
                {['round', 'floor', 'ceil', 'abs', 'min', 'max', 'sqrt', 'ln', 'exp'].map((fn) => (
                  <option key={fn} value={fn}>{fn}</option>
                ))}
              </select>
            </div>

            {(config.fn as string) === 'round' && (
              <div>
                <label className="block text-xs text-gray-500 mb-1">Places</label>
                <input
                  className="w-full text-xs border border-gray-300 rounded px-2 py-1"
                  value={((config.args as Record<string, string> | undefined)?.places) ?? '18'}
                  onChange={(e) => updateFunctionArg('places', e.target.value)}
                  placeholder="18"
                />
              </div>
            )}
          </>
        )}

        {nodeType === 'tableLookup' && (
          <>
            <div>
              <label className="block text-xs text-gray-500 mb-1">{t('table.title')}</label>
              <select
                className="w-full text-xs border border-gray-300 rounded px-2 py-1"
                value={(config.tableId as string) ?? ''}
                onChange={(e) => updateConfig('tableId', e.target.value)}
              >
                <option value="">— {t('table.selectTable')} —</option>
                {tables.map((tbl) => (
                  <option key={tbl.id} value={tbl.id}>
                    {tbl.name} ({tbl.tableType})
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label className="block text-xs text-gray-500 mb-1">Key Columns</label>
              {((config.keyColumns as string[] | undefined) ?? ['key']).map((col, i, arr) => (
                <div key={i} className="flex gap-1 mb-1">
                  <input
                    className="flex-1 text-xs border border-gray-300 rounded px-2 py-1"
                    value={col}
                    onChange={(e) => {
                      const next = [...arr]
                      next[i] = e.target.value
                      updateConfig('keyColumns', next)
                    }}
                  />
                  {arr.length > 1 && (
                    <button
                      className="text-xs text-red-500 px-1 hover:text-red-700"
                      onClick={() => updateConfig('keyColumns', arr.filter((_, j) => j !== i))}
                    >
                      ✕
                    </button>
                  )}
                </div>
              ))}
              <button
                className="mt-0.5 text-xs text-indigo-600 hover:text-indigo-800"
                onClick={() => {
                  const existing = (config.keyColumns as string[] | undefined) ?? ['key']
                  updateConfig('keyColumns', [...existing, `key${existing.length}`])
                }}
              >
                + Add key column
              </button>
            </div>
            <div>
              <label className="block text-xs text-gray-500 mb-1">Column</label>
              <input
                className="w-full text-xs border border-gray-300 rounded px-2 py-1"
                value={(config.column as string) ?? ''}
                onChange={(e) => updateConfig('column', e.target.value)}
              />
            </div>
          </>
        )}

        {nodeType === 'subFormula' && (
          <>
            <div>
              <label className="block text-xs text-gray-500 mb-1">{t('formula.id')}</label>
              <select
                className="w-full text-xs border border-gray-300 rounded px-2 py-1"
                value={selectedFormulaId}
                onChange={(e) => {
                  const nextFormulaId = e.target.value
                  const nextFormula = formulas.find((formula) => formula.id === nextFormulaId)
                  onChange(node.id, {
                    ...node.data,
                    config: {
                      ...config,
                      formulaId: nextFormulaId,
                      formulaName: nextFormula?.name ?? '',
                      version: undefined,
                    },
                  })
                }}
              >
                <option value="">Select formula</option>
                {formulas
                  .filter((formula) => formula.id !== currentFormulaId)
                  .map((formula) => (
                    <option key={formula.id} value={formula.id}>
                      {formula.name} ({formula.id})
                    </option>
                  ))}
              </select>
            </div>
            <div>
              <label className="block text-xs text-gray-500 mb-1">{t('version.versions')}</label>
              <select
                className="w-full text-xs border border-gray-300 rounded px-2 py-1"
                value={config.version === undefined || config.version === null ? '' : String(config.version)}
                onChange={(e) =>
                  updateConfig('version', e.target.value ? Number(e.target.value) : undefined)
                }
                disabled={!selectedFormulaId}
              >
                <option value="">{t('version.published')}</option>
                {formulaVersions.map((version) => (
                  <option key={version.version} value={version.version}>
                    v{version.version} ({version.state})
                  </option>
                ))}
              </select>
            </div>
          </>
        )}

        {nodeType === 'conditional' && (
          <div>
            <label className="block text-xs text-gray-500 mb-1">Comparator</label>
            <select
              className="w-full text-xs border border-gray-300 rounded px-2 py-1"
              value={(config.comparator as string) ?? 'gt'}
              onChange={(e) => updateConfig('comparator', e.target.value)}
            >
              <option value="eq">== Equal</option>
              <option value="ne">!= Not Equal</option>
              <option value="gt">&gt; Greater</option>
              <option value="ge">&gt;= Greater/Equal</option>
              <option value="lt">&lt; Less</option>
              <option value="le">&lt;= Less/Equal</option>
            </select>
          </div>
        )}

        {nodeType === 'aggregate' && (
          <div>
            <label className="block text-xs text-gray-500 mb-1">Aggregation</label>
            <select
              className="w-full text-xs border border-gray-300 rounded px-2 py-1"
              value={(config.fn as string) ?? 'sum'}
              onChange={(e) => updateConfig('fn', e.target.value)}
            >
              <option value="sum">Sum</option>
              <option value="avg">Average</option>
              <option value="count">Count</option>
              <option value="product">Product</option>
            </select>
          </div>
        )}

        {nodeType === 'loop' && (
          <>
            <div>
              <label className="block text-xs text-gray-500 mb-1">{t('editor.bodyFormula')}</label>
              <select
                className={`w-full text-xs border rounded px-2 py-1 ${!selectedFormulaId ? 'border-red-400 bg-red-50' : 'border-gray-300'}`}
                value={selectedFormulaId}
                onChange={(e) => {
                  const nextFormulaId = e.target.value
                  const nextFormula = formulas.find((f) => f.id === nextFormulaId)
                  onChange(node.id, {
                    ...node.data,
                    config: {
                      ...config,
                      formulaId: nextFormulaId,
                      formulaName: nextFormula?.name ?? '',
                      version: undefined,
                    },
                  })
                }}
              >
                <option value="">— {t('editor.selectFormula')} —</option>
                {formulas
                  .filter((f) => f.id !== currentFormulaId)
                  .map((f) => (
                    <option key={f.id} value={f.id}>
                      {f.name} ({f.id})
                    </option>
                  ))}
              </select>
            </div>
            <div>
              <label className="block text-xs text-gray-500 mb-1">{t('version.versions')}</label>
              <select
                className="w-full text-xs border border-gray-300 rounded px-2 py-1"
                value={config.version === undefined || config.version === null ? '' : String(config.version)}
                onChange={(e) =>
                  updateConfig('version', e.target.value ? Number(e.target.value) : undefined)
                }
                disabled={!selectedFormulaId}
              >
                <option value="">{t('version.published')}</option>
                {formulaVersions.map((v) => (
                  <option key={v.version} value={v.version}>
                    v{v.version} ({v.state})
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label className="block text-xs text-gray-500 mb-1">{t('editor.iterator')}</label>
              <input
                className="w-full text-xs border border-gray-300 rounded px-2 py-1 font-mono"
                value={(config.iterator as string) ?? 't'}
                placeholder="t"
                onChange={(e) => updateConfig('iterator', e.target.value)}
              />
            </div>
            <div>
              <label className="block text-xs text-gray-500 mb-1">{t('editor.aggregation')}</label>
              <select
                className="w-full text-xs border border-gray-300 rounded px-2 py-1"
                value={(config.aggregation as string) ?? 'sum'}
                onChange={(e) => updateConfig('aggregation', e.target.value)}
              >
                <option value="sum">Sum</option>
                <option value="avg">Average</option>
                <option value="count">Count</option>
                <option value="product">Product</option>
                <option value="min">Min</option>
                <option value="max">Max</option>
                <option value="last">Last</option>
                <option value="fold">Fold (accumulate)</option>
              </select>
            </div>
            {(config.aggregation as string) === 'fold' && (
              <>
                <div>
                  <label className="block text-xs text-gray-500 mb-1">Accumulator Variable</label>
                  <input
                    className="w-full text-xs border border-gray-300 rounded px-2 py-1 font-mono"
                    value={(config.accumulatorVar as string) ?? ''}
                    onChange={(e) => updateConfig('accumulatorVar', e.target.value)}
                    placeholder="V"
                  />
                </div>
                <div>
                  <label className="block text-xs text-gray-500 mb-1">Initial Value</label>
                  <input
                    className="w-full text-xs border border-gray-300 rounded px-2 py-1 font-mono"
                    value={(config.initValue as string) ?? ''}
                    onChange={(e) => updateConfig('initValue', e.target.value)}
                    placeholder="0"
                  />
                </div>
              </>
            )}
            <div className="flex items-center gap-2">
              <input
                type="checkbox"
                id="inclusiveEnd"
                checked={(config.inclusiveEnd as boolean) !== false}
                onChange={(e) => updateConfig('inclusiveEnd', e.target.checked)}
              />
              <label htmlFor="inclusiveEnd" className="text-xs text-gray-600">
                {t('editor.inclusiveEnd')}
              </label>
            </div>
            <div>
              <label className="block text-xs text-gray-500 mb-1">{t('editor.maxIterations')}</label>
              <input
                type="number"
                className="w-full text-xs border border-gray-300 rounded px-2 py-1"
                value={(config.maxIterations as number | undefined) ?? ''}
                placeholder="1000"
                min={1}
                onChange={(e) =>
                  updateConfig('maxIterations', e.target.value ? Number(e.target.value) : undefined)
                }
              />
            </div>
          </>
        )}
      </div>
    </div>
  )
}
