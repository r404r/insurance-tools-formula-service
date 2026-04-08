/**
 * Specialized inner-content components for each formula node type.
 * These render only the visual content; handles are managed by FormulaNode.
 */

const COMPARATOR_SYMBOLS: Record<string, string> = {
  gt: '>',
  lt: '<',
  ge: '≥',
  le: '≤',
  eq: '=',
  ne: '≠',
}

const OP_DISPLAY: Record<string, string> = {
  add: '+',
  subtract: '−',
  multiply: '×',
  divide: '÷',
  power: '^',
  modulo: '%',
}

const DATA_TYPE_LABEL: Record<string, string> = {
  decimal: 'num',
  string: 'str',
  boolean: 'bool',
}

// ---------------------------------------------------------------------------
// Variable
// ---------------------------------------------------------------------------

export function VariableInner({ config }: { config: Record<string, unknown> }) {
  const name = String(config.name ?? '')
  const dataType = String(config.dataType ?? 'decimal')
  const typeLabel = DATA_TYPE_LABEL[dataType] ?? dataType

  return (
    <div className="flex flex-col items-start gap-1 min-w-[100px]">
      <span className="rounded bg-blue-200 px-1.5 py-0.5 text-[9px] font-bold uppercase tracking-wide text-blue-700">
        {typeLabel}
      </span>
      <span className="text-[13px] font-bold leading-tight text-blue-900 max-w-[140px] truncate">
        {name || <span className="italic text-blue-400">unnamed</span>}
      </span>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Constant
// ---------------------------------------------------------------------------

export function ConstantInner({ config }: { config: Record<string, unknown> }) {
  const value = String(config.value ?? '?')
  return (
    <span className="font-mono text-[15px] font-bold text-amber-900">
      {value}
    </span>
  )
}

// ---------------------------------------------------------------------------
// Operator
// ---------------------------------------------------------------------------

export function OperatorInner({ config }: { config: Record<string, unknown> }) {
  const symbol = OP_DISPLAY[config.op as string] ?? String(config.op ?? '?')
  return (
    <span className="text-[26px] font-bold leading-none select-none text-pink-700">
      {symbol}
    </span>
  )
}

// ---------------------------------------------------------------------------
// Function
// ---------------------------------------------------------------------------

export function FunctionInner({ config }: { config: Record<string, unknown> }) {
  const fn = String(config.fn ?? '?')
  const args = config.args as Record<string, string> | undefined
  const places = args?.places
  return (
    <span className="font-mono text-[13px] font-bold text-green-800">
      {fn}{places ? `(${places})` : '()'}
    </span>
  )
}

// ---------------------------------------------------------------------------
// Sub-formula
// ---------------------------------------------------------------------------

export function SubFormulaInner({ config }: { config: Record<string, unknown> }) {
  const name = String(config.formulaName ?? config.formulaId ?? '').trim()
  return (
    <div className="flex flex-col items-center gap-0.5 min-w-[100px]">
      <span className="text-[9px] font-bold uppercase tracking-wide text-indigo-500">
        sub-formula
      </span>
      <span className="max-w-[140px] truncate text-[12px] font-semibold italic text-indigo-900">
        {name || <span className="text-indigo-300">(unset)</span>}
      </span>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Table lookup
// ---------------------------------------------------------------------------

export function TableLookupInner({ config }: { config: Record<string, unknown> }) {
  const column = String(config.column ?? '?')
  return (
    <div className="flex flex-col items-center gap-0.5">
      <span className="text-[10px] text-purple-500">▤ lookup</span>
      <span className="max-w-[120px] truncate font-mono text-[12px] font-bold text-purple-900">
        .{column}
      </span>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Conditional
// ---------------------------------------------------------------------------

export function ConditionalInner({ config }: { config: Record<string, unknown> }) {
  const comparator = String(config.comparator ?? 'gt')
  const symbol = COMPARATOR_SYMBOLS[comparator] ?? comparator
  return (
    <div className="flex flex-col items-center gap-0.5">
      <span className="text-[9px] font-bold uppercase tracking-wide text-orange-500">if</span>
      <span className="text-[22px] font-bold leading-none text-orange-800">{symbol}</span>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Aggregate
// ---------------------------------------------------------------------------

export function AggregateInner({ config }: { config: Record<string, unknown> }) {
  const fn = String(config.fn ?? '?')
  return (
    <div className="flex flex-col items-center gap-0.5">
      <span className="text-[18px] font-bold leading-none text-teal-700">Σ</span>
      <span className="text-[11px] font-semibold text-teal-900">{fn}</span>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Dispatcher
// ---------------------------------------------------------------------------

export function NodeVariantInner({
  nodeType,
  config,
}: {
  nodeType: string
  config: Record<string, unknown>
}) {
  switch (nodeType) {
    case 'variable':
      return <VariableInner config={config} />
    case 'constant':
      return <ConstantInner config={config} />
    case 'operator':
      return <OperatorInner config={config} />
    case 'function':
      return <FunctionInner config={config} />
    case 'subFormula':
      return <SubFormulaInner config={config} />
    case 'tableLookup':
      return <TableLookupInner config={config} />
    case 'conditional':
      return <ConditionalInner config={config} />
    case 'aggregate':
      return <AggregateInner config={config} />
    default:
      return <span className="text-[13px] font-semibold text-slate-700">{nodeType}</span>
  }
}
