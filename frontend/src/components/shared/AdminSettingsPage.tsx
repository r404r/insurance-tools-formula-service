import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { getSettings, updateSettings } from '../../api/settings'

export default function AdminSettingsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [maxCalcs, setMaxCalcs] = useState<string>('')
  const [saved, setSaved] = useState(false)

  const { data, isLoading, error } = useQuery({
    queryKey: ['admin-settings'],
    queryFn: getSettings,
  })

  useEffect(() => {
    if (data) setMaxCalcs(String(data.maxConcurrentCalcs))
  }, [data])

  const updateMutation = useMutation({
    mutationFn: updateSettings,
    onSuccess: (d) => {
      queryClient.setQueryData(['admin-settings'], d)
      setSaved(true)
      setTimeout(() => setSaved(false), 3000)
    },
  })

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    const n = parseInt(maxCalcs, 10)
    if (isNaN(n) || n < 0) return
    updateMutation.mutate({ maxConcurrentCalcs: n })
  }

  return (
    <div className="mx-auto max-w-2xl px-6 py-8">
      <div className="mb-6">
        <h1 className="text-2xl font-bold text-gray-900">{t('adminSettings.title')}</h1>
        <p className="mt-2 text-sm text-gray-500">{t('adminSettings.subtitle')}</p>
      </div>

      {isLoading ? (
        <div className="py-6 text-center text-gray-400">{t('common.loading')}</div>
      ) : error ? (
        <div className="py-6 text-center text-red-500">{t('common.error')}</div>
      ) : data ? (
        <form onSubmit={handleSubmit}>
          <div className="rounded-2xl border border-gray-200 bg-white p-6 shadow-sm">
            <h2 className="mb-5 text-lg font-semibold text-gray-900">
              {t('adminSettings.engineSection')}
            </h2>

            <div className="space-y-5">
              {/* Max concurrent calcs */}
              <div>
                <label className="mb-1 block text-sm font-medium text-gray-700">
                  {t('adminSettings.maxConcurrentCalcs')}
                </label>
                <p className="mb-2 text-xs text-gray-500">
                  {t('adminSettings.maxConcurrentCalcsHint')}
                </p>
                <input
                  type="number"
                  min={0}
                  max={10000}
                  value={maxCalcs}
                  onChange={(e) => setMaxCalcs(e.target.value)}
                  className="w-40 rounded-lg border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-indigo-500 focus:outline-none focus:ring-1 focus:ring-indigo-500"
                />
                <span className="ml-2 text-xs text-gray-400">
                  {t('adminSettings.zeroMeansUnlimited')}
                </span>
              </div>
            </div>

            <div className="mt-6 flex items-center gap-3">
              <button
                type="submit"
                disabled={updateMutation.isPending}
                className="rounded-lg bg-indigo-600 px-4 py-2 text-sm font-medium text-white shadow-sm transition hover:bg-indigo-700 disabled:opacity-50"
              >
                {updateMutation.isPending ? t('common.loading') : t('adminSettings.save')}
              </button>
              {saved && (
                <span className="text-sm text-green-600">{t('adminSettings.saveSuccess')}</span>
              )}
              {updateMutation.isError && (
                <span className="text-sm text-red-500">{t('common.error')}</span>
              )}
            </div>
          </div>
        </form>
      ) : null}
    </div>
  )
}
