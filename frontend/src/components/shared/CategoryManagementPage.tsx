import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createCategory, deleteCategory, listCategories, updateCategory } from '../../api/categories'
import { useAuthStore } from '../../store/authStore'
import type { Category } from '../../types/formula'

interface CategoryFormState {
  slug: string
  name: string
  description: string
  color: string
  sortOrder: number
}

const DEFAULT_COLOR = '#2563eb'

function slugify(value: string) {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

function toFormState(category: Category): CategoryFormState {
  return {
    slug: category.slug,
    name: category.name,
    description: category.description,
    color: category.color,
    sortOrder: category.sortOrder,
  }
}

export default function CategoryManagementPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const user = useAuthStore((state) => state.user)
  const isAdmin = user?.role === 'admin'
  const [createForm, setCreateForm] = useState<CategoryFormState>({
    slug: '',
    name: '',
    description: '',
    color: DEFAULT_COLOR,
    sortOrder: 0,
  })
  const [slugTouched, setSlugTouched] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editingForm, setEditingForm] = useState<CategoryFormState | null>(null)
  const [message, setMessage] = useState<string | null>(null)

  const { data: categories = [], isLoading, error } = useQuery({
    queryKey: ['categories'],
    queryFn: () => listCategories().then((response) => response.categories ?? []),
  })

  const categoryCount = useMemo(() => categories.length, [categories])

  const createMutation = useMutation({
    mutationFn: createCategory,
    onSuccess: async () => {
      setMessage(t('category.createSuccess'))
      setCreateForm({
        slug: '',
        name: '',
        description: '',
        color: DEFAULT_COLOR,
        sortOrder: 0,
      })
      setSlugTouched(false)
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['categories'] }),
        queryClient.invalidateQueries({ queryKey: ['formulas'] }),
      ])
    },
    onError: (err: Error) => {
      setMessage(err.message)
    },
  })

  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: CategoryFormState }) =>
      updateCategory(id, {
        name: data.name,
        description: data.description,
        color: data.color,
        sortOrder: data.sortOrder,
      }),
    onSuccess: async () => {
      setMessage(t('category.updateSuccess'))
      setEditingId(null)
      setEditingForm(null)
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['categories'] }),
        queryClient.invalidateQueries({ queryKey: ['formulas'] }),
      ])
    },
    onError: (err: Error) => {
      setMessage(err.message)
    },
  })

  const deleteMutation = useMutation({
    mutationFn: deleteCategory,
    onSuccess: async () => {
      setMessage(t('category.deleteSuccess'))
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['categories'] }),
        queryClient.invalidateQueries({ queryKey: ['formulas'] }),
      ])
    },
    onError: (err: Error) => {
      setMessage(err.message)
    },
  })

  if (!isAdmin) {
    return (
      <div className="mx-auto max-w-4xl px-6 py-10">
        <div className="rounded-xl border border-amber-200 bg-amber-50 px-5 py-4 text-sm text-amber-800">
          {t('category.adminOnly')}
        </div>
      </div>
    )
  }

  function handleCreateNameChange(value: string) {
    setCreateForm((current) => ({
      ...current,
      name: value,
      slug: slugTouched ? current.slug : slugify(value),
    }))
  }

  function handleCreate() {
    setMessage(null)
    createMutation.mutate({
      slug: createForm.slug.trim(),
      name: createForm.name.trim(),
      description: createForm.description.trim(),
      color: createForm.color,
      sortOrder: createForm.sortOrder,
    })
  }

  function startEditing(category: Category) {
    setMessage(null)
    setEditingId(category.id)
    setEditingForm(toFormState(category))
  }

  function handleUpdate() {
    if (!editingId || !editingForm) {
      return
    }

    setMessage(null)
    updateMutation.mutate({
      id: editingId,
      data: {
        ...editingForm,
        name: editingForm.name.trim(),
        description: editingForm.description.trim(),
      },
    })
  }

  function handleDelete(category: Category) {
    if (!window.confirm(t('category.deleteConfirm', { name: category.name }))) {
      return
    }

    setMessage(null)
    deleteMutation.mutate(category.id)
  }

  return (
    <div className="mx-auto max-w-6xl px-6 py-8">
      <div className="mb-6 flex items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">{t('category.title')}</h1>
          <p className="mt-2 text-sm text-gray-500">
            {t('category.subtitle', { count: categoryCount })}
          </p>
        </div>
      </div>

      {message && (
        <div className="mb-4 rounded-lg border border-gray-200 bg-white px-4 py-3 text-sm text-gray-700 shadow-sm">
          {message}
        </div>
      )}

      <div className="mb-8 rounded-2xl border border-gray-200 bg-white p-6 shadow-sm">
        <h2 className="mb-4 text-lg font-semibold text-gray-900">{t('category.create')}</h2>
        <div className="grid gap-4 md:grid-cols-2">
          <div>
            <label className="mb-1 block text-sm font-medium text-gray-700">{t('category.name')}</label>
            <input
              value={createForm.name}
              onChange={(e) => handleCreateNameChange(e.target.value)}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
            />
          </div>
          <div>
            <label className="mb-1 block text-sm font-medium text-gray-700">{t('category.slug')}</label>
            <input
              value={createForm.slug}
              onChange={(e) => {
                setSlugTouched(true)
                setCreateForm((current) => ({ ...current, slug: e.target.value }))
              }}
              placeholder={t('category.slugHint')}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
            />
          </div>
          <div>
            <label className="mb-1 block text-sm font-medium text-gray-700">{t('category.color')}</label>
            <div className="flex gap-3">
              <input
                type="color"
                value={createForm.color}
                onChange={(e) => setCreateForm((current) => ({ ...current, color: e.target.value }))}
                className="h-10 w-14 rounded-lg border border-gray-300 bg-white p-1"
              />
              <input
                value={createForm.color}
                onChange={(e) => setCreateForm((current) => ({ ...current, color: e.target.value }))}
                className="flex-1 rounded-lg border border-gray-300 px-3 py-2 text-sm"
              />
            </div>
          </div>
          <div>
            <label className="mb-1 block text-sm font-medium text-gray-700">{t('category.sortOrder')}</label>
            <input
              type="number"
              value={createForm.sortOrder}
              onChange={(e) =>
                setCreateForm((current) => ({
                  ...current,
                  sortOrder: Number.parseInt(e.target.value || '0', 10),
                }))
              }
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
            />
          </div>
          <div className="md:col-span-2">
            <label className="mb-1 block text-sm font-medium text-gray-700">
              {t('category.description')}
            </label>
            <textarea
              value={createForm.description}
              onChange={(e) => setCreateForm((current) => ({ ...current, description: e.target.value }))}
              rows={3}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
            />
          </div>
        </div>
        <div className="mt-4 flex justify-end">
          <button
            onClick={handleCreate}
            disabled={createMutation.isPending}
            className="rounded-lg bg-indigo-600 px-4 py-2 text-sm font-semibold text-white transition hover:bg-indigo-700 disabled:opacity-50"
          >
            {createMutation.isPending ? t('common.loading') : t('category.create')}
          </button>
        </div>
      </div>

      {isLoading ? (
        <div className="py-12 text-center text-gray-500">{t('common.loading')}</div>
      ) : error ? (
        <div className="py-12 text-center text-red-500">{t('common.error')}</div>
      ) : (
        <div className="grid gap-4">
          {categories.map((category) => {
            const isEditing = editingId === category.id && editingForm

            return (
              <div
                key={category.id}
                className="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm"
              >
                {isEditing ? (
                  <div className="grid gap-4 md:grid-cols-2">
                    <div>
                      <label className="mb-1 block text-sm font-medium text-gray-700">
                        {t('category.name')}
                      </label>
                      <input
                        value={editingForm.name}
                        onChange={(e) =>
                          setEditingForm((current) =>
                            current ? { ...current, name: e.target.value } : current
                          )
                        }
                        className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
                      />
                    </div>
                    <div>
                      <label className="mb-1 block text-sm font-medium text-gray-700">
                        {t('category.slug')}
                      </label>
                      <input
                        value={editingForm.slug}
                        disabled
                        className="w-full rounded-lg border border-gray-200 bg-gray-50 px-3 py-2 text-sm text-gray-500"
                      />
                    </div>
                    <div>
                      <label className="mb-1 block text-sm font-medium text-gray-700">
                        {t('category.color')}
                      </label>
                      <div className="flex gap-3">
                        <input
                          type="color"
                          value={editingForm.color}
                          onChange={(e) =>
                            setEditingForm((current) =>
                              current ? { ...current, color: e.target.value } : current
                            )
                          }
                          className="h-10 w-14 rounded-lg border border-gray-300 bg-white p-1"
                        />
                        <input
                          value={editingForm.color}
                          onChange={(e) =>
                            setEditingForm((current) =>
                              current ? { ...current, color: e.target.value } : current
                            )
                          }
                          className="flex-1 rounded-lg border border-gray-300 px-3 py-2 text-sm"
                        />
                      </div>
                    </div>
                    <div>
                      <label className="mb-1 block text-sm font-medium text-gray-700">
                        {t('category.sortOrder')}
                      </label>
                      <input
                        type="number"
                        value={editingForm.sortOrder}
                        onChange={(e) =>
                          setEditingForm((current) =>
                            current
                              ? {
                                  ...current,
                                  sortOrder: Number.parseInt(e.target.value || '0', 10),
                                }
                              : current
                          )
                        }
                        className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
                      />
                    </div>
                    <div className="md:col-span-2">
                      <label className="mb-1 block text-sm font-medium text-gray-700">
                        {t('category.description')}
                      </label>
                      <textarea
                        value={editingForm.description}
                        onChange={(e) =>
                          setEditingForm((current) =>
                            current ? { ...current, description: e.target.value } : current
                          )
                        }
                        rows={3}
                        className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
                      />
                    </div>
                    <div className="md:col-span-2 flex justify-end gap-3">
                      <button
                        onClick={() => {
                          setEditingId(null)
                          setEditingForm(null)
                        }}
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
                  <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
                    <div>
                      <div className="flex flex-wrap items-center gap-3">
                        <h3 className="text-lg font-semibold text-gray-900">{category.name}</h3>
                        <span
                          className="rounded-full px-2.5 py-0.5 text-xs font-medium"
                          style={{
                            color: category.color,
                            backgroundColor: `${category.color}18`,
                          }}
                        >
                          {category.slug}
                        </span>
                        <span className="rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-500">
                          {t('category.sortOrder')}: {category.sortOrder}
                        </span>
                      </div>
                      {category.description && (
                        <p className="mt-2 text-sm text-gray-500">{category.description}</p>
                      )}
                    </div>
                    <div className="flex gap-3">
                      <button
                        onClick={() => startEditing(category)}
                        className="rounded-lg border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 transition hover:bg-gray-50"
                      >
                        {t('common.edit')}
                      </button>
                      <button
                        onClick={() => handleDelete(category)}
                        disabled={deleteMutation.isPending}
                        className="rounded-lg border border-red-200 px-4 py-2 text-sm font-medium text-red-600 transition hover:bg-red-50 disabled:opacity-50"
                      >
                        {deleteMutation.isPending ? t('common.loading') : t('common.delete')}
                      </button>
                    </div>
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
