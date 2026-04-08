import { Handle, Position, type NodeProps } from '@xyflow/react'
import { NODE_COLORS, getInputPorts, type FormulaNodeData } from './nodePresentation'
import { NodeVariantInner } from './nodeVariants'

function PortLabel({ side, top, label }: { side: 'left' | 'right'; top: string; label: string }) {
  return (
    <span
      className={`absolute text-[10px] font-semibold text-slate-500 ${side === 'left' ? 'left-2' : 'right-2'}`}
      style={{ top, transform: 'translateY(-50%)' }}
    >
      {label}
    </span>
  )
}

export default function FormulaNode({ data, selected }: NodeProps) {
  const nodeData = data as unknown as FormulaNodeData
  const nodeType = nodeData.nodeType
  const config = nodeData.config ?? {}
  const colors = NODE_COLORS[nodeType] ?? { bg: '#f3f4f6', border: '#9ca3af' }
  const inputPorts = getInputPorts(nodeType, config)
  const isOperator = nodeType === 'operator'

  const containerClass = isOperator
    ? 'relative flex items-center justify-center rounded-full shadow-sm'
    : 'relative min-w-[108px] rounded-lg px-7 py-4 text-center shadow-sm'

  const containerStyle = isOperator
    ? {
        width: 72,
        height: 72,
        background: colors.bg,
        border: `2px solid ${selected ? '#2563eb' : colors.border}`,
      }
    : {
        background: colors.bg,
        border: `2px solid ${selected ? '#2563eb' : colors.border}`,
      }

  return (
    <div className={containerClass} style={containerStyle}>
      {inputPorts.map((port) => (
        <div key={port.id}>
          <PortLabel side="left" top={port.top} label={port.label} />
          <Handle
            type="target"
            id={port.id}
            position={Position.Left}
            style={{ top: port.top, width: 10, height: 10, background: colors.border, border: '1px solid white' }}
          />
        </div>
      ))}

      <div className="pointer-events-none flex items-center justify-center">
        <NodeVariantInner nodeType={nodeType} config={config} />
      </div>

      <PortLabel side="right" top="50%" label="Out" />
      <Handle
        type="source"
        id="out"
        position={Position.Right}
        style={{ top: '50%', width: 10, height: 10, background: colors.border, border: '1px solid white' }}
      />
    </div>
  )
}
