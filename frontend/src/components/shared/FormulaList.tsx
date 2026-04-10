import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query'
import { api, getToken } from '../../api/client'
import { useAuthStore } from '../../store/authStore'
import { listCategories } from '../../api/categories'
import TemplateGallery from './TemplateGallery'
import type { Category, Formula, InsuranceDomain } from '../../types/formula'

const PAGE_SIZE = 20

export default function FormulaList() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const user = useAuthStore((s) => s.user)

  const [search, setSearch] = useState('')
  const [domainFilter, setDomainFilter] = useState<InsuranceDomain | 'all'>('all')
  const [page, setPage] = useState(1)
  const [showCreateModal, setShowCreateModal] = useState(false)
  const [showTemplateGallery, setShowTemplateGallery] = useState(false)
  const [newName, setNewName] = useState('')
  const [newDomain, setNewDomain] = useState<InsuranceDomain>('')
  const [newDescription, setNewDescription] = useState('')

  const isEditor = user?.role === 'editor' || user?.role === 'admin'
  const isAdmin = user?.role === 'admin'

  const { data: categories = [] } = useQuery({
    queryKey: ['categories'],
    queryFn: () => listCategories().then((response) => response.categories ?? []),
  })

  const categoryMap = useMemo(
    () => new Map(categories.map((category) => [category.slug, category])),
    [categories]
  )

  const { data, isLoading, error } = useQuery({
    queryKey: ['formulas', search, domainFilter, page],
    queryFn: () => {
      const params = new URLSearchParams()
      if (search) params.set('search', search)
      if (domainFilter !== 'all') params.set('domain', domainFilter)
      params.set('limit', String(PAGE_SIZE))
      params.set('offset', String((page - 1) * PAGE_SIZE))
      return api.get<{ formulas: Formula[]; total: number; limit: number; offset: number }>(
        `/formulas?${params.toString()}`
      )
    },
    placeholderData: keepPreviousData,
  })

  const formulas = data?.formulas ?? []
  const total = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  // Clamp page when total shrinks (e.g. after delete or narrowed search).
  useEffect(() => {
    if (page > totalPages) setPage(totalPages)
  }, [totalPages, page])

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.delete(`/formulas/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['formulas'] })
    },
  })

  const handleDelete = (f: Formula, e: React.MouseEvent) => {
    e.stopPropagation()
    if (window.confirm(t('formula.deleteConfirm', { name: f.name }))) {
      deleteMutation.mutate(f.id)
    }
  }

  // ── Copy ──
  const [copySource, setCopySource] = useState<Formula | null>(null)
  const [copyName, setCopyName] = useState('')
  const [copyDescription, setCopyDescription] = useState('')

  const copyMutation = useMutation({
    mutationFn: (d: { id: string; name: string; description: string }) =>
      api.post<Formula>(`/formulas/${d.id}/copy`, { name: d.name, description: d.description }),
    onSuccess: (formula) => {
      queryClient.invalidateQueries({ queryKey: ['formulas'] })
      setCopySource(null)
      setCopyName('')
      setCopyDescription('')
      navigate(`/formulas/${formula.id}`)
    },
  })

  const handleCopyClick = (f: Formula, e: React.MouseEvent) => {
    e.stopPropagation()
    setCopySource(f)
    setCopyName(`${f.name} ${t('formula.copySuffix')}`)
    setCopyDescription(f.description ?? '')
  }

  const handleCopyConfirm = () => {
    if (!copySource || !copyName.trim()) return
    copyMutation.mutate({
      id: copySource.id,
      name: copyName.trim(),
      description: copyDescription,
    })
  }

  const createMutation = useMutation({
    mutationFn: (d: { name: string; domain: InsuranceDomain; description: string }) =>
      api.post<Formula>('/formulas', d),
    onSuccess: (formula) => {
      queryClient.invalidateQueries({ queryKey: ['formulas'] })
      setShowCreateModal(false)
      setNewName('')
      setNewDomain(categories[0]?.slug ?? '')
      setNewDescription('')
      navigate(`/formulas/${formula.id}`)
    },
  })

  // ── Export / Import ──
  const [importResult, setImportResult] = useState<{ imported: { id: string; name: string }[]; errors: { name: string; error: string }[] } | null>(null)

  async function handleExport(ids: string[], filename: string) {
    if (ids.length === 0) return
    try {
      const token = getToken()
      const res = await fetch('/api/v1/formulas/export', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...(token ? { Authorization: `Bearer ${token}` } : {}),
        },
        body: JSON.stringify({ ids }),
      })
      if (!res.ok) {
        window.alert(t('common.error'))
        return
      }
      // Detect partial exports (backend silently skips missing/broken formulas).
      const requested = parseInt(res.headers.get('X-Export-Requested') ?? '0', 10)
      const exported = parseInt(res.headers.get('X-Export-Exported') ?? '0', 10)
      if (requested > 0 && exported < requested) {
        window.alert(t('formula.exportPartial', { requested, exported }))
      }
      const blob = await res.blob()
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = filename
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      URL.revokeObjectURL(url)
    } catch {
      window.alert(t('common.error'))
    }
  }

  const exportAllMutation = useMutation({
    mutationFn: async () => {
      // Fetch all matching formulas up to 500 to get their IDs.
      const params = new URLSearchParams()
      if (search) params.set('search', search)
      if (domainFilter !== 'all') params.set('domain', domainFilter)
      params.set('limit', '500')
      params.set('offset', '0')
      const r = await api.get<{ formulas: Formula[] }>(`/formulas?${params.toString()}`)
      const ids = (r.formulas ?? []).map((f) => f.id)
      await handleExport(ids, `formulas-export-${new Date().toISOString().slice(0, 10)}.json`)
    },
  })

  const sanitizeFilename = (s: string): string => {
    // Strip reserved characters, control chars (U+0000-U+001F, U+007F), and leading dots.
    // eslint-disable-next-line no-control-regex
    const cleaned = s.replace(/[\\/:*?"<>|\x00-\x1f\x7f]/g, '_').replace(/^\.+/, '_')
    const trimmed = cleaned.slice(0, 120).trim()
    return trimmed || 'formula'
  }

  const handleSingleExport = (f: Formula, e: React.MouseEvent) => {
    e.stopPropagation()
    handleExport([f.id], `${sanitizeFilename(f.name)}.json`)
  }

  const importFileRef = useRef<HTMLInputElement | null>(null)

  const importMutation = useMutation({
    mutationFn: async (file: File) => {
      const text = await file.text()
      const body = JSON.parse(text)
      return api.post<{ imported: { id: string; name: string }[]; errors: { name: string; error: string }[] }>(
        '/formulas/import',
        body,
      )
    },
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: ['formulas'] })
      setImportResult(result)
    },
    onError: () => {
      setImportResult({ imported: [], errors: [{ name: '', error: t('formula.importParseError') }] })
    },
  })

  const handleImportClick = () => {
    importFileRef.current?.click()
  }

  const handleImportFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (file) importMutation.mutate(file)
    e.target.value = '' // reset so same file can be re-selected
  }

  useEffect(() => {
    if (categories.length === 0) {
      setNewDomain('')
      if (domainFilter !== 'all') setDomainFilter('all')
      return
    }
    setNewDomain((current) =>
      current && categories.some((category) => category.slug === current)
        ? current
        : categories[0].slug
    )
    if (domainFilter !== 'all' && !categories.some((category) => category.slug === domainFilter)) {
      setDomainFilter('all')
    }
  }, [categories, domainFilter])

  function handleCreate() {
    if (!newName.trim() || !newDomain) return
    createMutation.mutate({ name: newName, domain: newDomain, description: newDescription })
  }

  function renderCategoryBadge(categorySlug: string) {
    const category = categoryMap.get(categorySlug)
    const color = category?.color ?? '#6366f1'
    const label = category?.name ?? categorySlug
    return (
      <span
        className="inline-block rounded-full px-2.5 py-0.5 text-xs font-medium"
        style={{ color, backgroundColor: `${color}18` }}
      >
        {label}
      </span>
    )
  }

  // Build a compact page number list: always show first, last, current ±2, with … gaps.
  function pageNumbers(): Array<number | '…'> {
    if (totalPages <= 7) return Array.from({ length: totalPages }, (_, i) => i + 1)
    const nums = new Set<number>([1, totalPages, page, page - 1, page + 1, page - 2, page + 2])
    const sorted = [...nums].filter((n) => n >= 1 && n <= totalPages).sort((a, b) => a - b)
    const result: Array<number | '…'> = []
    for (let i = 0; i < sorted.length; i++) {
      if (i > 0 && sorted[i] - sorted[i - 1] > 1) result.push('…')
      result.push(sorted[i])
    }
    return result
  }

  return (
    <div className="mx-auto max-w-6xl px-6 py-8">
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-bold text-gray-900">{t('formula.list')}</h1>
        <div className="flex items-center gap-3">
          {isAdmin && (
            <button
              onClick={() => navigate('/categories')}
              className="rounded-lg border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 transition hover:bg-gray-50"
            >
              {t('formula.manageCategories')}
            </button>
          )}
          <button
            onClick={() => exportAllMutation.mutate()}
            disabled={exportAllMutation.isPending || formulas.length === 0}
            className="rounded-lg border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 transition hover:bg-gray-50 disabled:opacity-50"
          >
            {exportAllMutation.isPending ? t('common.loading') : t('formula.exportAll', { count: total })}
          </button>
          {isEditor && (
            <>
              <button
                onClick={handleImportClick}
                disabled={importMutation.isPending}
                className="rounded-lg border border-emerald-300 px-4 py-2 text-sm font-medium text-emerald-600 transition hover:bg-emerald-50 disabled:opacity-50"
              >
                {importMutation.isPending ? t('common.loading') : t('formula.import')}
              </button>
              <input
                ref={importFileRef}
                type="file"
                accept=".json,application/json"
                onChange={handleImportFileChange}
                className="hidden"
              />
              <button
                onClick={() => setShowTemplateGallery(true)}
                className="rounded-lg border border-indigo-300 px-4 py-2 text-sm font-medium text-indigo-600 transition hover:bg-indigo-50"
              >
                {t('template.fromTemplate')}
              </button>
              <button
                onClick={() => setShowCreateModal(true)}
                className="rounded-lg bg-indigo-600 px-4 py-2 text-sm font-semibold text-white transition hover:bg-indigo-700"
              >
                {t('formula.create')}
              </button>
            </>
          )}
        </div>
      </div>

      <div className="mb-4">
        <input
          type="text"
          value={search}
          onChange={(e) => { setSearch(e.target.value); setPage(1) }}
          placeholder={t('formula.search')}
          className="w-full max-w-md rounded-lg border border-gray-300 px-4 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-indigo-500 focus:outline-none focus:ring-2 focus:ring-indigo-200"
        />
      </div>

      <div className="mb-6 flex flex-wrap gap-1 rounded-lg bg-gray-100 p-1">
        {(['all', ...categories.map((category) => category.slug)] as Array<InsuranceDomain | 'all'>).map((d) => (
          <button
            key={d}
            onClick={() => { setDomainFilter(d); setPage(1) }}
            className={`rounded-md px-4 py-2 text-sm font-medium transition ${
              domainFilter === d
                ? 'bg-white text-indigo-600 shadow-sm'
                : 'text-gray-600 hover:text-gray-900'
            }`}
          >
            {d === 'all' ? t('formula.all') : categoryMap.get(d)?.name ?? d}
          </button>
        ))}
      </div>

      {isLoading ? (
        <div className="py-12 text-center text-gray-500">{t('common.loading')}</div>
      ) : error ? (
        <div className="py-12 text-center text-red-500">{t('common.error')}</div>
      ) : formulas.length === 0 ? (
        <div className="py-12 text-center text-gray-400">{t('common.noData')}</div>
      ) : (
        <>
          <div className="overflow-hidden rounded-lg border border-gray-200 bg-white shadow-sm">
            <table className="w-full text-left text-sm">
              <thead className="border-b border-gray-200 bg-gray-50">
                <tr>
                  <th className="px-6 py-3 font-medium text-gray-600">{t('formula.name')}</th>
                  <th className="px-6 py-3 font-medium text-gray-600">{t('formula.id')}</th>
                  {domainFilter === 'all' && (
                    <th className="px-6 py-3 font-medium text-gray-600">{t('formula.domain')}</th>
                  )}
                  <th className="px-6 py-3 font-medium text-gray-600">{t('formula.description')}</th>
                  <th className="px-6 py-3 font-medium text-gray-600">{t('formula.createdAt')}</th>
                  <th className="px-6 py-3 font-medium text-gray-600">{t('user.actions')}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {formulas.map((f) => (
                  <tr
                    key={f.id}
                    onClick={() => navigate(`/formulas/${f.id}`)}
                    className="cursor-pointer transition hover:bg-gray-50"
                  >
                    <td className="px-6 py-4 font-medium text-gray-900">{f.name}</td>
                    <td className="px-6 py-4 font-mono text-xs text-gray-500">{f.id}</td>
                    {domainFilter === 'all' && (
                      <td className="px-6 py-4">{renderCategoryBadge(f.domain)}</td>
                    )}
                    <td className="px-6 py-4 text-gray-500">{f.description}</td>
                    <td className="px-6 py-4 text-gray-400">
                      {new Date(f.createdAt).toLocaleDateString()}
                    </td>
                    <td className="px-6 py-4">
                      <div className="flex items-center gap-3">
                        <button
                          onClick={(e) => handleSingleExport(f, e)}
                          className="text-xs text-gray-500 hover:text-gray-800 transition"
                        >
                          {t('formula.export')}
                        </button>
                        {isEditor && (
                          <button
                            onClick={(e) => handleCopyClick(f, e)}
                            className="text-xs text-indigo-500 hover:text-indigo-700 transition"
                          >
                            {t('formula.copy')}
                          </button>
                        )}
                        {isAdmin && (
                          <button
                            onClick={(e) => handleDelete(f, e)}
                            className="text-xs text-red-500 hover:text-red-700 transition"
                          >
                            {t('formula.delete')}
                          </button>
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {/* Pagination bar — only rendered when there is more than one page */}
          {totalPages > 1 && (
            <div className="mt-4 flex items-center justify-between text-sm text-gray-500">
              <span>{t('formula.totalCount', { count: total })}</span>

              <div className="flex items-center gap-1">
                <button
                  onClick={() => setPage((p) => Math.max(1, p - 1))}
                  disabled={page === 1}
                  className="rounded px-2 py-1 transition hover:bg-gray-100 disabled:opacity-40"
                >
                  ‹
                </button>

                {pageNumbers().map((n, idx) =>
                  n === '…' ? (
                    <span key={`ellipsis-${idx}`} className="px-2">
                      …
                    </span>
                  ) : (
                    <button
                      key={n}
                      onClick={() => setPage(n)}
                      className={`min-w-[2rem] rounded px-2 py-1 transition ${
                        n === page
                          ? 'bg-indigo-600 font-semibold text-white'
                          : 'hover:bg-gray-100'
                      }`}
                    >
                      {n}
                    </button>
                  )
                )}

                <button
                  onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                  disabled={page === totalPages}
                  className="rounded px-2 py-1 transition hover:bg-gray-100 disabled:opacity-40"
                >
                  ›
                </button>
              </div>
            </div>
          )}
        </>
      )}

      {showTemplateGallery && (
        <TemplateGallery onClose={() => setShowTemplateGallery(false)} />
      )}

      {showCreateModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40">
          <div className="w-full max-w-md rounded-xl bg-white p-6 shadow-xl">
            <h2 className="mb-4 text-lg font-bold text-gray-900">{t('formula.createTitle')}</h2>

            <div className="space-y-4">
              <div>
                <label className="mb-1 block text-sm font-medium text-gray-700">
                  {t('formula.name')}
                </label>
                <input
                  type="text"
                  value={newName}
                  onChange={(e) => setNewName(e.target.value)}
                  className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 focus:border-indigo-500 focus:outline-none focus:ring-2 focus:ring-indigo-200"
                />
              </div>

              <div>
                <label className="mb-1 block text-sm font-medium text-gray-700">
                  {t('formula.domain')}
                </label>
                <select
                  value={newDomain}
                  onChange={(e) => setNewDomain(e.target.value as InsuranceDomain)}
                  disabled={categories.length === 0}
                  className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 focus:border-indigo-500 focus:outline-none focus:ring-2 focus:ring-indigo-200"
                >
                  {categories.map((category: Category) => (
                    <option key={category.id} value={category.slug}>
                      {category.name}
                    </option>
                  ))}
                </select>
                {categories.length === 0 && (
                  <p className="mt-2 text-xs text-amber-600">{t('formula.noCategory')}</p>
                )}
              </div>

              <div>
                <label className="mb-1 block text-sm font-medium text-gray-700">
                  {t('formula.description')}
                </label>
                <textarea
                  value={newDescription}
                  onChange={(e) => setNewDescription(e.target.value)}
                  rows={3}
                  className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 focus:border-indigo-500 focus:outline-none focus:ring-2 focus:ring-indigo-200"
                />
              </div>
            </div>

            <div className="mt-6 flex justify-end gap-3">
              <button
                onClick={() => setShowCreateModal(false)}
                className="rounded-lg border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 transition hover:bg-gray-50"
              >
                {t('formula.cancel')}
              </button>
              <button
                onClick={handleCreate}
                disabled={createMutation.isPending || !newName.trim() || !newDomain}
                className="rounded-lg bg-indigo-600 px-4 py-2 text-sm font-semibold text-white transition hover:bg-indigo-700 disabled:opacity-50"
              >
                {createMutation.isPending ? t('common.loading') : t('formula.create')}
              </button>
            </div>
          </div>
        </div>
      )}

      {copySource && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40">
          <div className="w-full max-w-md rounded-xl bg-white p-6 shadow-xl">
            <h2 className="mb-1 text-lg font-bold text-gray-900">{t('formula.copyTitle')}</h2>
            <p className="mb-4 text-xs text-gray-500">
              {t('formula.copyHint', { name: copySource.name })}
            </p>

            <div className="space-y-4">
              <div>
                <label className="mb-1 block text-sm font-medium text-gray-700">
                  {t('formula.name')}
                </label>
                <input
                  type="text"
                  value={copyName}
                  onChange={(e) => setCopyName(e.target.value)}
                  autoFocus
                  className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 focus:border-indigo-500 focus:outline-none focus:ring-2 focus:ring-indigo-200"
                />
              </div>

              <div>
                <label className="mb-1 block text-sm font-medium text-gray-700">
                  {t('formula.description')}
                </label>
                <textarea
                  value={copyDescription}
                  onChange={(e) => setCopyDescription(e.target.value)}
                  rows={3}
                  className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 focus:border-indigo-500 focus:outline-none focus:ring-2 focus:ring-indigo-200"
                />
              </div>
            </div>

            <div className="mt-6 flex justify-end gap-3">
              <button
                onClick={() => setCopySource(null)}
                disabled={copyMutation.isPending}
                className="rounded-lg border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 transition hover:bg-gray-50 disabled:opacity-50"
              >
                {t('formula.cancel')}
              </button>
              <button
                onClick={handleCopyConfirm}
                disabled={copyMutation.isPending || !copyName.trim()}
                className="rounded-lg bg-indigo-600 px-4 py-2 text-sm font-semibold text-white transition hover:bg-indigo-700 disabled:opacity-50"
              >
                {copyMutation.isPending ? t('common.loading') : t('formula.copy')}
              </button>
            </div>
            {copyMutation.isError && (
              <p className="mt-3 text-sm text-red-600">{t('common.error')}</p>
            )}
          </div>
        </div>
      )}

      {importResult && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40">
          <div className="w-full max-w-lg rounded-xl bg-white p-6 shadow-xl">
            <h2 className="mb-4 text-lg font-bold text-gray-900">{t('formula.importResult')}</h2>

            <div className="mb-4 grid grid-cols-2 gap-4">
              <div className="rounded-lg bg-emerald-50 p-3">
                <p className="text-xs text-emerald-600">{t('formula.importSuccess')}</p>
                <p className="mt-1 text-2xl font-bold text-emerald-700">{importResult.imported.length}</p>
              </div>
              <div className="rounded-lg bg-red-50 p-3">
                <p className="text-xs text-red-600">{t('formula.importFailure')}</p>
                <p className="mt-1 text-2xl font-bold text-red-700">{importResult.errors.length}</p>
              </div>
            </div>

            {importResult.imported.length > 0 && (
              <div className="mb-3 max-h-40 overflow-y-auto rounded border border-emerald-100 bg-emerald-50/30 p-2 text-xs">
                {importResult.imported.map((item) => (
                  <div key={item.id} className="py-0.5 text-emerald-800">✓ {item.name}</div>
                ))}
              </div>
            )}

            {importResult.errors.length > 0 && (
              <div className="mb-3 max-h-40 overflow-y-auto rounded border border-red-100 bg-red-50/30 p-2 text-xs">
                {importResult.errors.map((err, i) => (
                  <div key={i} className="py-0.5 text-red-800">
                    ✗ {err.name || `#${i}`}: {err.error}
                  </div>
                ))}
              </div>
            )}

            <div className="mt-6 flex justify-end">
              <button
                onClick={() => setImportResult(null)}
                className="rounded-lg bg-indigo-600 px-4 py-2 text-sm font-semibold text-white transition hover:bg-indigo-700"
              >
                {t('common.confirm')}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
