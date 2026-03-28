import { useTranslation } from 'react-i18next'
import type { NodeType } from '../../types/formula'

const nodeTypes: { type: NodeType; icon: string }[] = [
  { type: 'variable', icon: 'x' },
  { type: 'constant', icon: '#' },
  { type: 'operator', icon: '+-' },
  { type: 'function', icon: 'f(x)' },
  { type: 'subFormula', icon: '{}' },
  { type: 'tableLookup', icon: '[]' },
  { type: 'conditional', icon: '?' },
  { type: 'aggregate', icon: 'E' },
]

export default function NodePalette() {
  const { t } = useTranslation()

  const onDragStart = (e: React.DragEvent, nodeType: NodeType) => {
    e.dataTransfer.setData('application/reactflow-type', nodeType)
    e.dataTransfer.effectAllowed = 'move'
  }

  return (
    <div className="w-48 border-r border-gray-200 bg-gray-50 p-3 overflow-y-auto">
      <h3 className="text-sm font-semibold text-gray-600 mb-3">{t('editor.nodeTypes')}</h3>
      <div className="space-y-2">
        {nodeTypes.map(({ type, icon }) => (
          <div
            key={type}
            draggable
            onDragStart={(e) => onDragStart(e, type)}
            className="flex items-center gap-2 px-3 py-2 bg-white rounded-md border border-gray-200 cursor-grab hover:border-blue-400 hover:shadow-sm transition-all text-sm"
          >
            <span className="w-8 h-8 flex items-center justify-center bg-blue-50 text-blue-600 rounded text-xs font-mono font-bold">
              {icon}
            </span>
            <span className="text-gray-700">{t(`node.${type}`)}</span>
          </div>
        ))}
      </div>
    </div>
  )
}
