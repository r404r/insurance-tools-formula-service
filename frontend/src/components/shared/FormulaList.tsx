import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import { useAuthStore } from '../../store/authStore'
import { listCategories } from '../../api/categories'
import type { Category, Formula, InsuranceDomain } from '../../types/formula'

export default function FormulaList() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const user = useAuthStore((s) => s.user)

  const [search, setSearch] = useState('')
  const [domainFilter, setDomainFilter] = useState<InsuranceDomain | 'all'>('all')
  const [showCreateModal, setShowCreateModal] = useState(false)
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

  const { data: formulas = [], isLoading, error } = useQuery({
    queryKey: ['formulas', search, domainFilter],
    queryFn: () => {
      const params = new URLSearchParams()
      if (search) params.set('search', search)
      if (domainFilter !== 'all') params.set('domain', domainFilter)
      const qs = params.toString()
      return api.get<{ formulas: Formula[]; total: number }>(`/formulas${qs ? `?${qs}` : ''}`).then((r) => r.formulas ?? [])
    },
  })

  const createMutation = useMutation({
    mutationFn: (data: { name: string; domain: InsuranceDomain; description: string }) =>
      api.post<Formula>('/formulas', data),
    onSuccess: (formula) => {
      queryClient.invalidateQueries({ queryKey: ['formulas'] })
      setShowCreateModal(false)
      setNewName('')
      setNewDomain(categories[0]?.slug ?? '')
      setNewDescription('')
      navigate(`/formulas/${formula.id}`)
    },
  })

  useEffect(() => {
    if (categories.length === 0) {
      setNewDomain('')
      if (domainFilter !== 'all') {
        setDomainFilter('all')
      }
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
        style={{
          color,
          backgroundColor: `${color}18`,
        }}
      >
        {label}
      </span>
    )
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
          {isEditor && (
            <button
              onClick={() => setShowCreateModal(true)}
              className="rounded-lg bg-indigo-600 px-4 py-2 text-sm font-semibold text-white transition hover:bg-indigo-700"
            >
              {t('formula.create')}
            </button>
          )}
        </div>
      </div>

      <div className="mb-4">
        <input
          type="text"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder={t('formula.search')}
          className="w-full max-w-md rounded-lg border border-gray-300 px-4 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-indigo-500 focus:outline-none focus:ring-2 focus:ring-indigo-200"
        />
      </div>

      <div className="mb-6 flex flex-wrap gap-1 rounded-lg bg-gray-100 p-1">
        {(['all', ...categories.map((category) => category.slug)] as Array<InsuranceDomain | 'all'>).map((d) => (
          <button
            key={d}
            onClick={() => setDomainFilter(d)}
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
        <div className="overflow-hidden rounded-lg border border-gray-200 bg-white shadow-sm">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-gray-200 bg-gray-50">
              <tr>
                <th className="px-6 py-3 font-medium text-gray-600">{t('formula.name')}</th>
                <th className="px-6 py-3 font-medium text-gray-600">{t('formula.id')}</th>
                <th className="px-6 py-3 font-medium text-gray-600">{t('formula.domain')}</th>
                <th className="px-6 py-3 font-medium text-gray-600">{t('formula.description')}</th>
                <th className="px-6 py-3 font-medium text-gray-600">{t('formula.createdAt')}</th>
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
                  <td className="px-6 py-4">{renderCategoryBadge(f.domain)}</td>
                  <td className="px-6 py-4 text-gray-500">{f.description}</td>
                  <td className="px-6 py-4 text-gray-400">
                    {new Date(f.createdAt).toLocaleDateString()}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
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
    </div>
  )
}
