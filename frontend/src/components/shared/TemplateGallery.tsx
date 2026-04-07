import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { useQuery, useMutation } from '@tanstack/react-query'
import { listTemplates } from '../../api/templates'
import { listCategories } from '../../api/categories'
import { api } from '../../api/client'
import type { Formula, FormulaGraph, InsuranceDomain, FormulaTemplate } from '../../types/formula'

interface Props {
  onClose: () => void
}

// Two-step flow: step 1 = browse, step 2 = confirm + name
type Step = 'browse' | 'confirm'

export default function TemplateGallery({ onClose }: Props) {
  const { t } = useTranslation()
  const navigate = useNavigate()

  const [domainTab, setDomainTab] = useState<InsuranceDomain | 'all'>('all')
  const [selected, setSelected] = useState<FormulaTemplate | null>(null)
  const [step, setStep] = useState<Step>('browse')
  const [newName, setNewName] = useState('')
  const [newDomain, setNewDomain] = useState<InsuranceDomain>('')
  const [newDescription, setNewDescription] = useState('')

  const { data: templates = [], isLoading } = useQuery({
    queryKey: ['templates'],
    queryFn: listTemplates,
  })

  const { data: categories = [] } = useQuery({
    queryKey: ['categories'],
    queryFn: () => listCategories().then((r) => r.categories ?? []),
  })

  // Create formula then immediately create a version with the template graph.
  const createMutation = useMutation({
    mutationFn: async ({
      name,
      domain,
      description,
      graph,
    }: {
      name: string
      domain: InsuranceDomain
      description: string
      graph: FormulaGraph
    }) => {
      const formula = await api.post<Formula>('/formulas', { name, domain, description })
      try {
        await api.post(`/formulas/${formula.id}/versions`, {
          graph,
          changeNote: t('template.initialVersionNote'),
        })
      } catch (err) {
        // Roll back the formula to avoid leaving an orphan record.
        await api.delete(`/formulas/${formula.id}`).catch(() => undefined)
        throw err
      }
      return formula
    },
    onSuccess: (formula) => {
      navigate(`/formulas/${formula.id}`)
    },
  })

  function selectTemplate(tpl: FormulaTemplate) {
    setSelected(tpl)
    setNewName(tpl.name)
    setNewDescription(tpl.description)
    // Match template domain to an existing category slug; leave empty if no match so
    // the user must explicitly pick — avoids silently filing under the wrong category.
    const matchedCat = categories.find((c) => c.slug === tpl.domain)
    setNewDomain(matchedCat?.slug ?? '')
    setStep('confirm')
  }

  function handleConfirm() {
    if (!selected || !newName.trim() || !newDomain) return
    createMutation.mutate({
      name: newName,
      domain: newDomain,
      description: newDescription,
      graph: selected.graph,
    })
  }

  const filtered =
    domainTab === 'all' ? templates : templates.filter((t) => t.domain === domainTab)

  const domainTabs: Array<InsuranceDomain | 'all'> = [
    'all',
    ...Array.from(new Set(templates.map((t) => t.domain))),
  ]

  function domainLabel(d: InsuranceDomain | 'all') {
    if (d === 'all') return t('formula.all')
    const cat = categories.find((c) => c.slug === d)
    if (cat) return cat.name
    return d
  }

  function domainColor(d: InsuranceDomain) {
    const cat = categories.find((c) => c.slug === d)
    return cat?.color ?? '#6366f1'
  }

  function nodeCountLabel(tpl: FormulaTemplate) {
    return t('template.nodeCount', { count: tpl.graph.nodes.length })
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40">
      <div className="flex h-[80vh] w-full max-w-3xl flex-col overflow-hidden rounded-xl bg-white shadow-xl">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-gray-200 px-6 py-4">
          <h2 className="text-lg font-bold text-gray-900">
            {step === 'browse' ? t('template.galleryTitle') : t('template.confirmTitle')}
          </h2>
          <button
            onClick={onClose}
            className="text-gray-400 transition hover:text-gray-600"
            aria-label="close"
          >
            ✕
          </button>
        </div>

        {step === 'browse' ? (
          <>
            {/* Domain tabs */}
            <div className="flex gap-1 border-b border-gray-100 bg-gray-50 px-6 py-2">
              {domainTabs.map((d) => (
                <button
                  key={d}
                  onClick={() => setDomainTab(d)}
                  className={`rounded-md px-3 py-1.5 text-sm font-medium transition ${
                    domainTab === d
                      ? 'bg-white text-indigo-600 shadow-sm'
                      : 'text-gray-600 hover:text-gray-900'
                  }`}
                >
                  {domainLabel(d)}
                </button>
              ))}
            </div>

            {/* Template cards */}
            <div className="flex-1 overflow-y-auto p-6">
              {isLoading ? (
                <div className="py-12 text-center text-gray-500">{t('common.loading')}</div>
              ) : filtered.length === 0 ? (
                <div className="py-12 text-center text-gray-400">{t('common.noData')}</div>
              ) : (
                <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                  {filtered.map((tpl) => {
                    const color = domainColor(tpl.domain)
                    return (
                      <div
                        key={tpl.id}
                        className="flex flex-col rounded-lg border border-gray-200 bg-white p-4 shadow-sm transition hover:border-indigo-300 hover:shadow-md"
                      >
                        <div className="mb-2 flex items-center justify-between">
                          <span className="font-semibold text-gray-900">{tpl.name}</span>
                          <span
                            className="rounded-full px-2 py-0.5 text-xs font-medium"
                            style={{ color, backgroundColor: `${color}18` }}
                          >
                            {domainLabel(tpl.domain)}
                          </span>
                        </div>
                        <p className="mb-3 flex-1 text-xs text-gray-500 line-clamp-3">
                          {tpl.description}
                        </p>
                        <div className="flex items-center justify-between">
                          <span className="text-xs text-gray-400">{nodeCountLabel(tpl)}</span>
                          <button
                            onClick={() => selectTemplate(tpl)}
                            className="rounded-md bg-indigo-600 px-3 py-1.5 text-xs font-semibold text-white transition hover:bg-indigo-700"
                          >
                            {t('template.use')}
                          </button>
                        </div>
                      </div>
                    )
                  })}
                </div>
              )}
            </div>
          </>
        ) : (
          /* Confirm step */
          <div className="flex-1 overflow-y-auto p-6">
            <p className="mb-4 text-sm text-gray-500">
              {t('template.confirmHint', { name: selected?.name })}
            </p>

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
                  {!newDomain && (
                    <option value="" disabled>
                      {t('formula.domain')} —
                    </option>
                  )}
                  {categories.map((cat) => (
                    <option key={cat.id} value={cat.slug}>
                      {cat.name}
                    </option>
                  ))}
                </select>
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

              {createMutation.isError && (
                <p className="text-sm text-red-500">
                  {(createMutation.error as Error)?.message ?? t('common.error')}
                </p>
              )}
            </div>

            <div className="mt-6 flex justify-end gap-3">
              <button
                onClick={() => setStep('browse')}
                className="rounded-lg border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 transition hover:bg-gray-50"
              >
                {t('template.back')}
              </button>
              <button
                onClick={handleConfirm}
                disabled={createMutation.isPending || !newName.trim() || !newDomain}
                className="rounded-lg bg-indigo-600 px-4 py-2 text-sm font-semibold text-white transition hover:bg-indigo-700 disabled:opacity-50"
              >
                {createMutation.isPending ? t('common.loading') : t('template.create')}
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
