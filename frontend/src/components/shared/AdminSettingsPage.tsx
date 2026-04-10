import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { getSettings, updateSettings } from '../../api/settings'
import { getCacheStats, clearCache } from '../../api/cache'
import { api } from '../../api/client'
import { useAuthStore } from '../../store/authStore'

export default function AdminSettingsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const user = useAuthStore((s) => s.user)
  const isAdmin = user?.role === 'admin'

  const [maxCalcs, setMaxCalcs] = useState<string>('')
  const [saved, setSaved] = useState(false)

  const { data, isLoading, error } = useQuery({
    queryKey: ['admin-settings'],
    queryFn: getSettings,
    enabled: isAdmin,
  })

  const {
    data: cacheData,
    isLoading: cacheLoading,
    error: cacheError,
    refetch: refetchCache,
  } = useQuery({
    queryKey: ['cache-stats'],
    queryFn: getCacheStats,
    refetchInterval: isAdmin ? 5000 : false,
    enabled: isAdmin,
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

  const clearMutation = useMutation({
    mutationFn: clearCache,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['cache-stats'] })
    },
  })

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    const n = parseInt(maxCalcs, 10)
    if (isNaN(n) || n < 0) return
    updateMutation.mutate({ maxConcurrentCalcs: n })
  }

  const usagePct =
    cacheData && cacheData.maxSize > 0
      ? Math.round((cacheData.size / cacheData.maxSize) * 100)
      : 0

  if (!isAdmin) {
    return (
      <div className="mx-auto max-w-2xl px-6 py-16 text-center text-gray-500">
        {t('user.adminOnly')}
      </div>
    )
  }

  return (
    <div className="mx-auto max-w-2xl px-6 py-8">
      <div className="mb-6">
        <h1 className="text-2xl font-bold text-gray-900">{t('adminSettings.title')}</h1>
        <p className="mt-2 text-sm text-gray-500">{t('adminSettings.subtitle')}</p>
      </div>

      <div className="space-y-6">
        {/* ── Calculation Engine ── */}
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

        {/* ── Cache Management ── */}
        <div className="rounded-2xl border border-gray-200 bg-white p-6 shadow-sm">
          <h2 className="mb-4 text-lg font-semibold text-gray-900">{t('cache.stats')}</h2>

          {cacheLoading ? (
            <div className="py-6 text-center text-gray-400">{t('common.loading')}</div>
          ) : cacheError ? (
            <div className="py-6 text-center text-red-500">{t('common.error')}</div>
          ) : cacheData ? (
            <div className="space-y-4">
              <div className="grid grid-cols-2 gap-4">
                <div className="rounded-lg bg-gray-50 p-4">
                  <p className="text-xs text-gray-500">{t('cache.entries')}</p>
                  <p className="mt-1 text-2xl font-bold text-gray-900">
                    {cacheData.size.toLocaleString()}
                  </p>
                </div>
                <div className="rounded-lg bg-gray-50 p-4">
                  <p className="text-xs text-gray-500">{t('cache.capacity')}</p>
                  <p className="mt-1 text-2xl font-bold text-gray-900">
                    {cacheData.maxSize.toLocaleString()}
                  </p>
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
                  onClick={() => refetchCache()}
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
                    disabled={clearMutation.isPending || cacheData.size === 0}
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

        <div className="rounded-lg border border-amber-100 bg-amber-50 px-4 py-3 text-sm text-amber-700">
          {t('cache.hint')}
        </div>

        {/* ── Seed Data Reset ── */}
        <SeedResetSection />
      </div>
    </div>
  )
}

function SeedResetSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const resetMutation = useMutation({
    mutationFn: () => api.post<{ message: string }>('/admin/reset-seed', {}),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['formulas'] })
      queryClient.invalidateQueries({ queryKey: ['tables'] })
    },
  })

  return (
    <div className="rounded-2xl border border-gray-200 bg-white p-6 shadow-sm">
      <h2 className="mb-4 text-lg font-semibold text-gray-900">{t('adminSettings.seedSection')}</h2>
      <p className="mb-4 text-sm text-gray-500">{t('adminSettings.seedHint')}</p>
      <button
        onClick={() => {
          if (window.confirm(t('adminSettings.seedResetConfirm'))) {
            resetMutation.mutate()
          }
        }}
        disabled={resetMutation.isPending}
        className="rounded-lg border border-orange-200 px-4 py-2 text-sm font-medium text-orange-600 transition hover:bg-orange-50 disabled:opacity-50"
      >
        {resetMutation.isPending ? t('common.loading') : t('adminSettings.seedReset')}
      </button>
      {resetMutation.isSuccess && (
        <p className="mt-2 text-sm text-green-600">{t('adminSettings.seedResetSuccess')}</p>
      )}
      {resetMutation.isError && (
        <p className="mt-2 text-sm text-red-500">{t('common.error')}</p>
      )}
    </div>
  )
}
