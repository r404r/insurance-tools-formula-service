import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import type { Node } from '@xyflow/react'
import { api } from '../../api/client'
import { listTables } from '../../api/tables'
import type { Formula, FormulaVersion, LookupTable } from '../../types/formula'

interface Props {
  node: Node | null
  onChange: (id: string, data: Record<string, unknown>) => void
  currentFormulaId?: string | null
}

export default function NodePropertiesPanel({ node, onChange, currentFormulaId }: Props) {
  const { t } = useTranslation()
  const config = ((node?.data.config as Record<string, unknown> | undefined) ?? {})
  const selectedFormulaId = String(config.formulaId ?? '')

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
          <input className="w-full text-xs bg-gray-100 rounded px-2 py-1" value={node.id} disabled />
        </div>
        <div>
          <label className="block text-xs text-gray-500 mb-1">Type</label>
          <input className="w-full text-xs bg-gray-100 rounded px-2 py-1" value={nodeType} disabled />
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
              <label className="block text-xs text-gray-500 mb-1">Lookup Key</label>
              <input
                className="w-full text-xs border border-gray-300 rounded px-2 py-1"
                value={(config.lookupKey as string) ?? ''}
                onChange={(e) => updateConfig('lookupKey', e.target.value)}
              />
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
      </div>
    </div>
  )
}
