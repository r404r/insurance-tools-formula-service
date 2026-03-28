import { useTranslation } from 'react-i18next'
import type { Node } from '@xyflow/react'

interface Props {
  node: Node | null
  onChange: (id: string, data: Record<string, unknown>) => void
}

export default function NodePropertiesPanel({ node, onChange }: Props) {
  const { t } = useTranslation()

  if (!node) {
    return (
      <div className="w-64 border-l border-gray-200 bg-gray-50 p-4 text-sm text-gray-400">
        {t('editor.properties')}: {t('common.noData')}
      </div>
    )
  }

  const nodeType = (node.data.nodeType as string) ?? node.type
  const config = (node.data.config as Record<string, unknown>) ?? {}

  const updateConfig = (key: string, value: unknown) => {
    onChange(node.id, { ...node.data, config: { ...config, [key]: value } })
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
        <div>
          <label className="block text-xs text-gray-500 mb-1">Label</label>
          <input
            className="w-full text-xs border border-gray-300 rounded px-2 py-1"
            value={(node.data.label as string) ?? ''}
            onChange={(e) => onChange(node.id, { ...node.data, label: e.target.value })}
          />
        </div>

        {nodeType === 'variable' && (
          <>
            <div>
              <label className="block text-xs text-gray-500 mb-1">{t('formula.name')}</label>
              <input
                className="w-full text-xs border border-gray-300 rounded px-2 py-1"
                value={(config.variableName as string) ?? ''}
                onChange={(e) => updateConfig('variableName', e.target.value)}
              />
            </div>
            <div>
              <label className="block text-xs text-gray-500 mb-1">Data Type</label>
              <select
                className="w-full text-xs border border-gray-300 rounded px-2 py-1"
                value={(config.dataType as string) ?? 'number'}
                onChange={(e) => updateConfig('dataType', e.target.value)}
              >
                <option value="number">Number</option>
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
              value={(config.operator as string) ?? 'add'}
              onChange={(e) => updateConfig('operator', e.target.value)}
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
          <div>
            <label className="block text-xs text-gray-500 mb-1">Function</label>
            <select
              className="w-full text-xs border border-gray-300 rounded px-2 py-1"
              value={(config.function as string) ?? 'round'}
              onChange={(e) => updateConfig('function', e.target.value)}
            >
              {['round', 'floor', 'ceil', 'abs', 'min', 'max', 'sqrt', 'ln', 'exp'].map((fn) => (
                <option key={fn} value={fn}>{fn}</option>
              ))}
            </select>
          </div>
        )}

        {nodeType === 'tableLookup' && (
          <>
            <div>
              <label className="block text-xs text-gray-500 mb-1">Table ID</label>
              <input
                className="w-full text-xs border border-gray-300 rounded px-2 py-1"
                value={(config.tableId as string) ?? ''}
                onChange={(e) => updateConfig('tableId', e.target.value)}
              />
            </div>
            <div>
              <label className="block text-xs text-gray-500 mb-1">Result Column</label>
              <input
                className="w-full text-xs border border-gray-300 rounded px-2 py-1"
                value={(config.resultColumn as string) ?? ''}
                onChange={(e) => updateConfig('resultColumn', e.target.value)}
              />
            </div>
          </>
        )}

        {nodeType === 'conditional' && (
          <div>
            <label className="block text-xs text-gray-500 mb-1">Comparator</label>
            <select
              className="w-full text-xs border border-gray-300 rounded px-2 py-1"
              value={(config.comparison as string) ?? 'gt'}
              onChange={(e) => updateConfig('comparison', e.target.value)}
            >
              <option value="eq">== Equal</option>
              <option value="ne">!= Not Equal</option>
              <option value="gt">&gt; Greater</option>
              <option value="gte">&gt;= Greater/Equal</option>
              <option value="lt">&lt; Less</option>
              <option value="lte">&lt;= Less/Equal</option>
            </select>
          </div>
        )}

        {nodeType === 'aggregate' && (
          <div>
            <label className="block text-xs text-gray-500 mb-1">Aggregation</label>
            <select
              className="w-full text-xs border border-gray-300 rounded px-2 py-1"
              value={(config.aggregation as string) ?? 'sum'}
              onChange={(e) => updateConfig('aggregation', e.target.value)}
            >
              <option value="sum">Sum</option>
              <option value="avg">Average</option>
              <option value="count">Count</option>
              <option value="min">Min</option>
              <option value="max">Max</option>
            </select>
          </div>
        )}
      </div>
    </div>
  )
}
