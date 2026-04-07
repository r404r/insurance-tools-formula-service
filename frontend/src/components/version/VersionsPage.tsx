import { useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { FormulaVersion, Formula } from '../../types/formula'
import VersionDiffModal from './VersionDiffModal'

const stateBadge: Record<string, string> = {
  draft: 'bg-yellow-100 text-yellow-700',
  published: 'bg-green-100 text-green-700',
  archived: 'bg-gray-100 text-gray-500',
}

export default function VersionsPage() {
  const { id } = useParams<{ id: string }>()
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [diffTarget, setDiffTarget] = useState<{ from: number; to: number } | null>(null)

  const { data: formula } = useQuery({
    queryKey: ['formula', id],
    queryFn: () => api.get<Formula>(`/formulas/${id}`),
    enabled: !!id,
  })

  const { data: versions, isLoading } = useQuery({
    queryKey: ['versions', id],
    queryFn: () => api.get<{ versions: FormulaVersion[] }>(`/formulas/${id}/versions`).then((r) => r.versions),
    enabled: !!id,
  })

  const updateState = useMutation({
    mutationFn: ({ ver, state }: { ver: number; state: string }) =>
      api.patch(`/formulas/${id}/versions/${ver}`, { state }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['versions', id] }),
  })

  return (
    <div className="max-w-4xl mx-auto p-6">
      <div className="flex items-center justify-between mb-6">
        <div className="flex items-center gap-3">
          <Link to={`/formulas/${id}`} className="text-gray-400 hover:text-gray-600">&larr;</Link>
          <h1 className="text-xl font-semibold">{formula?.name} - {t('version.versions')}</h1>
        </div>
      </div>

      {isLoading ? (
        <p className="text-gray-400">{t('common.loading')}</p>
      ) : !versions?.length ? (
        <p className="text-gray-400">{t('common.noData')}</p>
      ) : (
        <div className="space-y-3">
          {versions.map((v) => (
            <div key={v.version} className="bg-white rounded-lg border border-gray-200 p-4 flex items-center justify-between">
              <div className="flex items-center gap-4">
                <span className="text-lg font-mono font-bold text-gray-700">v{v.version}</span>
                <span className={`px-2 py-0.5 rounded text-xs font-medium ${stateBadge[v.state] ?? ''}`}>
                  {t(`version.${v.state}`)}
                </span>
                {v.changeNote && (
                  <span className="text-sm text-gray-500">{v.changeNote}</span>
                )}
                <span className="text-xs text-gray-400">{new Date(v.createdAt).toLocaleString()}</span>
              </div>
              <div className="flex items-center gap-2">
                {v.version > 1 && (
                  <button
                    onClick={() => setDiffTarget({ from: v.version - 1, to: v.version })}
                    className="text-xs bg-indigo-50 text-indigo-600 px-3 py-1 rounded border border-indigo-200 hover:bg-indigo-100"
                  >
                    {t('version.diff')}
                  </button>
                )}
                {v.state === 'draft' && (
                  <button
                    onClick={() => updateState.mutate({ ver: v.version, state: 'published' })}
                    className="text-xs bg-green-600 text-white px-3 py-1 rounded hover:bg-green-700"
                  >
                    {t('version.publish')}
                  </button>
                )}
                {(v.state === 'draft' || v.state === 'published') && (
                  <button
                    onClick={() => updateState.mutate({ ver: v.version, state: 'archived' })}
                    className="text-xs bg-gray-200 text-gray-700 px-3 py-1 rounded hover:bg-gray-300"
                  >
                    {t('version.archive')}
                  </button>
                )}
              </div>
            </div>
          ))}
        </div>
      )}

      {diffTarget && id && (
        <VersionDiffModal
          formulaId={id}
          fromVersion={diffTarget.from}
          toVersion={diffTarget.to}
          onClose={() => setDiffTarget(null)}
        />
      )}
    </div>
  )
}
