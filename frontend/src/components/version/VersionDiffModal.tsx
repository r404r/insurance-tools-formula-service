import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { getVersionDiff } from '../../api/versions'
import type { FormulaNode, FormulaEdge, NodeDiff } from '../../types/formula'

interface Props {
  formulaId: string
  fromVersion: number
  toVersion: number
  onClose: () => void
}

export default function VersionDiffModal({ formulaId, fromVersion, toVersion, onClose }: Props) {
  const { t } = useTranslation()

  const { data, isLoading, error } = useQuery({
    queryKey: ['version-diff', formulaId, fromVersion, toVersion],
    queryFn: () => getVersionDiff(formulaId, fromVersion, toVersion),
  })

  const totalChanges = data
    ? data.addedNodes.length + data.removedNodes.length + data.modifiedNodes.length +
      data.addedEdges.length + data.removedEdges.length
    : 0

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
      <div className="flex max-h-[90vh] w-full max-w-2xl flex-col rounded-2xl bg-white shadow-2xl">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-gray-200 px-6 py-4">
          <div>
            <h2 className="text-lg font-semibold text-gray-900">
              {t('diff.title', { from: fromVersion, to: toVersion })}
            </h2>
            {data && (
              <p className="mt-0.5 text-xs text-gray-500">
                {t('diff.summary', {
                  nodes: data.addedNodes.length + data.removedNodes.length + data.modifiedNodes.length,
                  edges: data.addedEdges.length + data.removedEdges.length,
                })}
              </p>
            )}
          </div>
          <button
            onClick={onClose}
            className="rounded-lg p-2 text-gray-400 hover:bg-gray-100 hover:text-gray-600"
          >
            ✕
          </button>
        </div>

        {/* Body */}
        <div className="flex-1 overflow-y-auto px-6 py-4">
          {isLoading && (
            <p className="py-8 text-center text-gray-400">{t('common.loading')}</p>
          )}
          {error && (
            <p className="py-8 text-center text-red-500">{t('common.error')}</p>
          )}
          {data && totalChanges === 0 && (
            <p className="py-8 text-center text-gray-400">{t('diff.noChanges')}</p>
          )}
          {data && totalChanges > 0 && (
            <div className="space-y-5">
              {/* Added nodes */}
              {data.addedNodes.length > 0 && (
                <Section
                  title={t('diff.addedNodes', { count: data.addedNodes.length })}
                  color="green"
                >
                  {data.addedNodes.map((n) => (
                    <NodeRow key={n.id} node={n} />
                  ))}
                </Section>
              )}

              {/* Removed nodes */}
              {data.removedNodes.length > 0 && (
                <Section
                  title={t('diff.removedNodes', { count: data.removedNodes.length })}
                  color="red"
                >
                  {data.removedNodes.map((n) => (
                    <NodeRow key={n.id} node={n} />
                  ))}
                </Section>
              )}

              {/* Modified nodes */}
              {data.modifiedNodes.length > 0 && (
                <Section
                  title={t('diff.modifiedNodes', { count: data.modifiedNodes.length })}
                  color="yellow"
                >
                  {data.modifiedNodes.map((nd) => (
                    <ModifiedNodeRow key={nd.nodeId} diff={nd} t={t} />
                  ))}
                </Section>
              )}

              {/* Edge changes */}
              {(data.addedEdges.length > 0 || data.removedEdges.length > 0) && (
                <Section
                  title={t('diff.edgeChanges', {
                    added: data.addedEdges.length,
                    removed: data.removedEdges.length,
                  })}
                  color="blue"
                >
                  {data.addedEdges.map((e, i) => (
                    <EdgeRow key={`add-${i}`} edge={e} added />
                  ))}
                  {data.removedEdges.map((e, i) => (
                    <EdgeRow key={`rem-${i}`} edge={e} added={false} />
                  ))}
                </Section>
              )}
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="border-t border-gray-200 px-6 py-3 text-right">
          <button
            onClick={onClose}
            className="rounded-lg bg-gray-100 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-200"
          >
            {t('common.cancel')}
          </button>
        </div>
      </div>
    </div>
  )
}

// ── Sub-components ──────────────────────────────────────────────────────────

const COLOR_CLASSES: Record<string, { border: string; bg: string; badge: string }> = {
  green:  { border: 'border-green-200',  bg: 'bg-green-50',  badge: 'bg-green-100 text-green-700' },
  red:    { border: 'border-red-200',    bg: 'bg-red-50',    badge: 'bg-red-100 text-red-700' },
  yellow: { border: 'border-yellow-200', bg: 'bg-yellow-50', badge: 'bg-yellow-100 text-yellow-700' },
  blue:   { border: 'border-blue-200',   bg: 'bg-blue-50',   badge: 'bg-blue-100 text-blue-700' },
}

function Section({
  title,
  color,
  children,
}: {
  title: string
  color: keyof typeof COLOR_CLASSES
  children: React.ReactNode
}) {
  const c = COLOR_CLASSES[color]
  return (
    <div className={`rounded-xl border ${c.border} ${c.bg} p-4`}>
      <div className="mb-3">
        <span className={`rounded-full px-2 py-0.5 text-xs font-semibold ${c.badge}`}>
          {title}
        </span>
      </div>
      <div className="space-y-2">{children}</div>
    </div>
  )
}

function NodeRow({ node }: { node: FormulaNode }) {
  return (
    <div className="flex items-start gap-3 rounded-lg bg-white/70 px-3 py-2 text-sm">
      <code className="font-mono text-xs text-gray-500">{node.id}</code>
      <span className="rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-600">{node.type}</span>
      <span className="ml-auto font-mono text-xs text-gray-500 break-all">
        {JSON.stringify(node.config)}
      </span>
    </div>
  )
}

function ModifiedNodeRow({ diff, t }: { diff: NodeDiff; t: (k: string) => string }) {
  return (
    <div className="rounded-lg bg-white/70 px-3 py-2 text-xs">
      <div className="mb-1 flex items-center gap-2">
        <code className="font-mono text-gray-500">{diff.nodeId}</code>
        <span className="rounded bg-gray-100 px-1.5 py-0.5 text-gray-600">{diff.after.type}</span>
      </div>
      <div className="grid grid-cols-2 gap-2">
        <div>
          <p className="mb-1 text-gray-400">{t('diff.before')}</p>
          <pre className="overflow-x-auto rounded bg-red-100 px-2 py-1 text-red-700">
            {JSON.stringify(diff.before.config, null, 2)}
          </pre>
        </div>
        <div>
          <p className="mb-1 text-gray-400">{t('diff.after')}</p>
          <pre className="overflow-x-auto rounded bg-green-100 px-2 py-1 text-green-700">
            {JSON.stringify(diff.after.config, null, 2)}
          </pre>
        </div>
      </div>
    </div>
  )
}

function EdgeRow({ edge, added }: { edge: FormulaEdge; added: boolean }) {
  return (
    <div className="flex items-center gap-2 rounded-lg bg-white/70 px-3 py-1.5 text-xs">
      <span className={`font-bold ${added ? 'text-green-600' : 'text-red-600'}`}>
        {added ? '+' : '−'}
      </span>
      <code className="font-mono text-gray-600">
        {edge.source}:{edge.sourcePort} → {edge.target}:{edge.targetPort}
      </code>
    </div>
  )
}
