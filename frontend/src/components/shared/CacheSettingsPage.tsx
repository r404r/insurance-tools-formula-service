import { useTranslation } from 'react-i18next'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { getCacheStats, clearCache } from '../../api/cache'
import { useAuthStore } from '../../store/authStore'

export default function CacheSettingsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const user = useAuthStore((s) => s.user)
  const isAdmin = user?.role === 'admin'

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['cache-stats'],
    queryFn: getCacheStats,
    refetchInterval: 5000,
  })

  const clearMutation = useMutation({
    mutationFn: clearCache,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['cache-stats'] })
    },
  })

  const usagePct = data && data.maxSize > 0
    ? Math.round((data.size / data.maxSize) * 100)
    : 0

  return (
    <div className="mx-auto max-w-2xl px-6 py-8">
      <div className="mb-6">
        <h1 className="text-2xl font-bold text-gray-900">{t('cache.title')}</h1>
        <p className="mt-2 text-sm text-gray-500">{t('cache.subtitle')}</p>
      </div>

      <div className="rounded-2xl border border-gray-200 bg-white p-6 shadow-sm">
        <h2 className="mb-4 text-lg font-semibold text-gray-900">{t('cache.stats')}</h2>

        {isLoading ? (
          <div className="py-6 text-center text-gray-400">{t('common.loading')}</div>
        ) : error ? (
          <div className="py-6 text-center text-red-500">{t('common.error')}</div>
        ) : data ? (
          <div className="space-y-4">
            <div className="grid grid-cols-2 gap-4">
              <div className="rounded-lg bg-gray-50 p-4">
                <p className="text-xs text-gray-500">{t('cache.entries')}</p>
                <p className="mt-1 text-2xl font-bold text-gray-900">{data.size.toLocaleString()}</p>
              </div>
              <div className="rounded-lg bg-gray-50 p-4">
                <p className="text-xs text-gray-500">{t('cache.capacity')}</p>
                <p className="mt-1 text-2xl font-bold text-gray-900">{data.maxSize.toLocaleString()}</p>
              </div>
            </div>

            <div>
              <div className="mb-1 flex justify-between text-xs text-gray-500">
                <span>{t('cache.usage')}</span>
                <span>{usagePct}%</span>
              </div>
              <div className="h-2 rounded-full bg-gray-100">
                <div
                  className="h-2 rounded-full bg-indigo-500 transition-all"
                  style={{ width: `${usagePct}%` }}
                />
              </div>
            </div>

            <div className="flex items-center gap-3 pt-2">
              <button
                onClick={() => refetch()}
                className="rounded-lg border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 transition hover:bg-gray-50"
              >
                {t('cache.refresh')}
              </button>

              {isAdmin && (
                <button
                  onClick={() => {
                    if (window.confirm(t('cache.clearConfirm'))) {
                      clearMutation.mutate()
                    }
                  }}
                  disabled={clearMutation.isPending || data.size === 0}
                  className="rounded-lg border border-red-200 px-4 py-2 text-sm font-medium text-red-600 transition hover:bg-red-50 disabled:opacity-50"
                >
                  {clearMutation.isPending ? t('common.loading') : t('cache.clear')}
                </button>
              )}
            </div>

            {clearMutation.isSuccess && (
              <p className="text-sm text-green-600">{t('cache.clearSuccess')}</p>
            )}
          </div>
        ) : null}
      </div>

      <div className="mt-4 rounded-lg border border-amber-100 bg-amber-50 px-4 py-3 text-sm text-amber-700">
        {t('cache.hint')}
      </div>
    </div>
  )
}
