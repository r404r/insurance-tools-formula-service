import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { listTables, createTable, updateTable, deleteTable } from '../../api/tables'
import { listCategories } from '../../api/categories'
import { useAuthStore } from '../../store/authStore'
import type { LookupTable } from '../../types/formula'

interface TableFormState {
  name: string
  domain: string
  tableType: string
  dataJson: string
}

const EMPTY_FORM: TableFormState = {
  name: '',
  domain: '',
  tableType: 'factor',
  dataJson: '[\n  {"key": "1", "value": "0.001"}\n]',
}

function tryParseJson(s: string): unknown | null {
  try {
    return JSON.parse(s)
  } catch {
    return null
  }
}

function DataPreview({ data }: { data: unknown }) {
  if (!Array.isArray(data) || data.length === 0) return null
  const first = data[0]
  if (!first || typeof first !== 'object') return null
  const keys = Object.keys(first as Record<string, unknown>)
  if (keys.length === 0) return null
  return (
    <div className="mt-3 overflow-auto rounded-lg border border-gray-200">
      <table className="w-full text-xs">
        <thead>
          <tr className="bg-gray-50">
            {keys.map((k) => (
              <th key={k} className="px-3 py-1.5 text-left font-semibold text-gray-600">
                {k}
              </th>
            ))}
          </tr>
        </thead>
        <tbody className="divide-y divide-gray-100">
          {(data as Record<string, unknown>[]).slice(0, 8).map((row, i) => (
            <tr key={i}>
              {keys.map((k) => (
                <td key={k} className="px-3 py-1 text-gray-700">
                  {String(row[k] ?? '')}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
      {data.length > 8 && (
        <p className="px-3 py-1.5 text-xs text-gray-400">…{data.length - 8} more rows</p>
      )}
    </div>
  )
}

export default function TableManagementPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const user = useAuthStore((s) => s.user)
  const canEdit = user?.role === 'admin' || user?.role === 'editor'

  const [createForm, setCreateForm] = useState<TableFormState>(EMPTY_FORM)
  const [jsonError, setJsonError] = useState<string | null>(null)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editingForm, setEditingForm] = useState<TableFormState | null>(null)
  const [editJsonError, setEditJsonError] = useState<string | null>(null)
  const [message, setMessage] = useState<string | null>(null)
  const [domainFilter, setDomainFilter] = useState('')

  const { data: categoriesData } = useQuery({
    queryKey: ['categories'],
    queryFn: () => listCategories().then((r) => r.categories ?? []),
  })
  const categories = categoriesData ?? []

  const { data, isLoading, error } = useQuery({
    queryKey: ['tables', domainFilter],
    queryFn: () => listTables(domainFilter || undefined).then((r) => r.tables ?? []),
  })
  const tables = data ?? []

  const createMutation = useMutation({
    mutationFn: createTable,
    onSuccess: async () => {
      setMessage(t('table.createSuccess'))
      setCreateForm(EMPTY_FORM)
      setJsonError(null)
      await queryClient.invalidateQueries({ queryKey: ['tables'] })
    },
    onError: (err: Error) => setMessage(err.message),
  })

  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: Parameters<typeof updateTable>[1] }) =>
      updateTable(id, data),
    onSuccess: async () => {
      setMessage(t('table.updateSuccess'))
      setEditingId(null)
      setEditingForm(null)
      await queryClient.invalidateQueries({ queryKey: ['tables'] })
    },
    onError: (err: Error) => setMessage(err.message),
  })

  const deleteMutation = useMutation({
    mutationFn: deleteTable,
    onSuccess: async () => {
      setMessage(t('table.deleteSuccess'))
      await queryClient.invalidateQueries({ queryKey: ['tables'] })
    },
    onError: (err: Error) => setMessage(err.message),
  })

  function handleCreate() {
    const parsed = tryParseJson(createForm.dataJson)
    if (parsed === null) {
      setJsonError(t('table.invalidJson'))
      return
    }
    setJsonError(null)
    setMessage(null)
    createMutation.mutate({
      name: createForm.name.trim(),
      domain: createForm.domain,
      tableType: createForm.tableType.trim(),
      data: parsed,
    })
  }

  function startEditing(table: LookupTable) {
    setMessage(null)
    setEditingId(table.id)
    setEditJsonError(null)
    setEditingForm({
      name: table.name,
      domain: table.domain,
      tableType: table.tableType,
      dataJson: JSON.stringify(table.data, null, 2),
    })
  }

  function handleUpdate() {
    if (!editingId || !editingForm) return
    const parsed = tryParseJson(editingForm.dataJson)
    if (parsed === null) {
      setEditJsonError(t('table.invalidJson'))
      return
    }
    setEditJsonError(null)
    setMessage(null)
    updateMutation.mutate({
      id: editingId,
      data: { name: editingForm.name.trim(), tableType: editingForm.tableType.trim(), data: parsed },
    })
  }

  function handleDelete(table: LookupTable) {
    if (!window.confirm(t('table.deleteConfirm', { name: table.name }))) return
    setMessage(null)
    deleteMutation.mutate(table.id)
  }

  return (
    <div className="mx-auto max-w-6xl px-6 py-8">
      <div className="mb-6 flex items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">{t('table.title')}</h1>
          <p className="mt-2 text-sm text-gray-500">
            {t('table.subtitle', { count: tables.length })}
          </p>
        </div>
        {/* Domain filter */}
        <select
          value={domainFilter}
          onChange={(e) => setDomainFilter(e.target.value)}
          className="rounded-lg border border-gray-300 px-3 py-2 text-sm"
        >
          <option value="">{t('formula.all')}</option>
          {categories.map((c) => (
            <option key={c.slug} value={c.slug}>{c.name}</option>
          ))}
        </select>
      </div>

      {message && (
        <div className="mb-4 rounded-lg border border-gray-200 bg-white px-4 py-3 text-sm text-gray-700 shadow-sm">
          {message}
        </div>
      )}

      {/* Create form (editors+) */}
      {canEdit && (
        <div className="mb-8 rounded-2xl border border-gray-200 bg-white p-6 shadow-sm">
          <h2 className="mb-4 text-lg font-semibold text-gray-900">{t('table.create')}</h2>
          <div className="grid gap-4 md:grid-cols-3">
            <div>
              <label className="mb-1 block text-sm font-medium text-gray-700">{t('table.name')}</label>
              <input
                value={createForm.name}
                onChange={(e) => setCreateForm((f) => ({ ...f, name: e.target.value }))}
                className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
              />
            </div>
            <div>
              <label className="mb-1 block text-sm font-medium text-gray-700">{t('table.domain')}</label>
              <select
                value={createForm.domain}
                onChange={(e) => setCreateForm((f) => ({ ...f, domain: e.target.value }))}
                className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
              >
                <option value="">— {t('table.selectDomain')} —</option>
                {categories.map((c) => (
                  <option key={c.slug} value={c.slug}>{c.name}</option>
                ))}
              </select>
            </div>
            <div>
              <label className="mb-1 block text-sm font-medium text-gray-700">{t('table.tableType')}</label>
              <input
                value={createForm.tableType}
                onChange={(e) => setCreateForm((f) => ({ ...f, tableType: e.target.value }))}
                placeholder="mortality / rating / factor"
                className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
              />
            </div>
            <div className="md:col-span-3">
              <label className="mb-1 block text-sm font-medium text-gray-700">
                {t('table.data')}
                <span className="ml-2 text-xs font-normal text-gray-400">{t('table.dataHint')}</span>
              </label>
              <textarea
                value={createForm.dataJson}
                onChange={(e) => {
                  setCreateForm((f) => ({ ...f, dataJson: e.target.value }))
                  setJsonError(null)
                }}
                rows={6}
                spellCheck={false}
                className="w-full rounded-lg border border-gray-300 px-3 py-2 font-mono text-xs"
              />
              {jsonError && <p className="mt-1 text-xs text-red-500">{jsonError}</p>}
              <DataPreview data={tryParseJson(createForm.dataJson)} />
            </div>
          </div>
          <div className="mt-4 flex justify-end">
            <button
              onClick={handleCreate}
              disabled={createMutation.isPending || !createForm.name || !createForm.domain}
              className="rounded-lg bg-indigo-600 px-4 py-2 text-sm font-semibold text-white transition hover:bg-indigo-700 disabled:opacity-50"
            >
              {createMutation.isPending ? t('common.loading') : t('table.create')}
            </button>
          </div>
        </div>
      )}

      {/* Table list */}
      {isLoading ? (
        <div className="py-12 text-center text-gray-500">{t('common.loading')}</div>
      ) : error ? (
        <div className="py-12 text-center text-red-500">{t('common.error')}</div>
      ) : (
        <div className="grid gap-4">
          {tables.map((table) => {
            const isEditing = editingId === table.id && editingForm
            return (
              <div key={table.id} className="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm">
                {isEditing ? (
                  <div className="grid gap-4">
                    <div className="grid gap-4 md:grid-cols-2">
                      <div>
                        <label className="mb-1 block text-sm font-medium text-gray-700">{t('table.name')}</label>
                        <input
                          value={editingForm.name}
                          onChange={(e) => setEditingForm((f) => f ? { ...f, name: e.target.value } : f)}
                          className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
                        />
                      </div>
                      <div>
                        <label className="mb-1 block text-sm font-medium text-gray-700">{t('table.tableType')}</label>
                        <input
                          value={editingForm.tableType}
                          onChange={(e) => setEditingForm((f) => f ? { ...f, tableType: e.target.value } : f)}
                          className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
                        />
                      </div>
                    </div>
                    <div>
                      <label className="mb-1 block text-sm font-medium text-gray-700">{t('table.data')}</label>
                      <textarea
                        value={editingForm.dataJson}
                        onChange={(e) => {
                          setEditingForm((f) => f ? { ...f, dataJson: e.target.value } : f)
                          setEditJsonError(null)
                        }}
                        rows={8}
                        spellCheck={false}
                        className="w-full rounded-lg border border-gray-300 px-3 py-2 font-mono text-xs"
                      />
                      {editJsonError && <p className="mt-1 text-xs text-red-500">{editJsonError}</p>}
                      <DataPreview data={tryParseJson(editingForm.dataJson)} />
                    </div>
                    <div className="flex justify-end gap-3">
                      <button
                        onClick={() => { setEditingId(null); setEditingForm(null) }}
                        className="rounded-lg border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 transition hover:bg-gray-50"
                      >
                        {t('common.cancel')}
                      </button>
                      <button
                        onClick={handleUpdate}
                        disabled={updateMutation.isPending}
                        className="rounded-lg bg-indigo-600 px-4 py-2 text-sm font-semibold text-white transition hover:bg-indigo-700 disabled:opacity-50"
                      >
                        {updateMutation.isPending ? t('common.loading') : t('common.save')}
                      </button>
                    </div>
                  </div>
                ) : (
                  <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
                    <div className="flex-1 min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <h3 className="text-base font-semibold text-gray-900">{table.name}</h3>
                        <span className="rounded bg-purple-50 px-2 py-0.5 text-xs text-purple-600">
                          {table.tableType}
                        </span>
                        <span className="rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-500">
                          {table.domain}
                        </span>
                        <span className="text-xs text-gray-400 font-mono">{table.id.slice(0, 8)}…</span>
                      </div>
                      <DataPreview data={table.data} />
                    </div>
                    {canEdit && (
                      <div className="flex gap-2 shrink-0">
                        <button
                          onClick={() => startEditing(table)}
                          className="rounded-lg border border-gray-300 px-3 py-1.5 text-sm font-medium text-gray-700 transition hover:bg-gray-50"
                        >
                          {t('common.edit')}
                        </button>
                        <button
                          onClick={() => handleDelete(table)}
                          disabled={deleteMutation.isPending}
                          className="rounded-lg border border-red-200 px-3 py-1.5 text-sm font-medium text-red-600 transition hover:bg-red-50 disabled:opacity-50"
                        >
                          {t('common.delete')}
                        </button>
                      </div>
                    )}
                  </div>
                )}
              </div>
            )
          })}
          {tables.length === 0 && (
            <div className="py-12 text-center text-gray-400">{t('common.noData')}</div>
          )}
        </div>
      )}
    </div>
  )
}
