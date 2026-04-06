import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { listUsers, updateUserRole, deleteUser } from '../../api/users'
import { useAuthStore } from '../../store/authStore'
import type { Role, User } from '../../types/formula'

const ROLES: Role[] = ['admin', 'editor', 'reviewer', 'viewer']

export default function UserManagementPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const currentUser = useAuthStore((s) => s.user)
  const isAdmin = currentUser?.role === 'admin'
  const [message, setMessage] = useState<string | null>(null)

  const { data, isLoading, error } = useQuery({
    queryKey: ['users'],
    queryFn: () => listUsers().then((r) => r.users ?? []),
    enabled: isAdmin,
  })

  const users = data ?? []

  const updateRoleMutation = useMutation({
    mutationFn: ({ id, role }: { id: string; role: Role }) => updateUserRole(id, role),
    onSuccess: async () => {
      setMessage(t('user.roleUpdated'))
      await queryClient.invalidateQueries({ queryKey: ['users'] })
    },
    onError: (err: Error) => {
      setMessage(err.message)
    },
  })

  const deleteMutation = useMutation({
    mutationFn: deleteUser,
    onSuccess: async () => {
      setMessage(t('user.deleted'))
      await queryClient.invalidateQueries({ queryKey: ['users'] })
    },
    onError: (err: Error) => {
      setMessage(err.message)
    },
  })

  if (!isAdmin) {
    return (
      <div className="mx-auto max-w-4xl px-6 py-10">
        <div className="rounded-xl border border-amber-200 bg-amber-50 px-5 py-4 text-sm text-amber-800">
          {t('user.adminOnly')}
        </div>
      </div>
    )
  }

  function handleRoleChange(user: User, role: Role) {
    setMessage(null)
    updateRoleMutation.mutate({ id: user.id, role })
  }

  function handleDelete(user: User) {
    if (!window.confirm(t('user.deleteConfirm', { username: user.username }))) return
    setMessage(null)
    deleteMutation.mutate(user.id)
  }

  return (
    <div className="mx-auto max-w-5xl px-6 py-8">
      <div className="mb-6">
        <h1 className="text-2xl font-bold text-gray-900">{t('user.title')}</h1>
        <p className="mt-2 text-sm text-gray-500">
          {t('user.subtitle', { count: users.length })}
        </p>
      </div>

      {message && (
        <div className="mb-4 rounded-lg border border-gray-200 bg-white px-4 py-3 text-sm text-gray-700 shadow-sm">
          {message}
        </div>
      )}

      {isLoading ? (
        <div className="py-12 text-center text-gray-500">{t('common.loading')}</div>
      ) : error ? (
        <div className="py-12 text-center text-red-500">{t('common.error')}</div>
      ) : (
        <div className="overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-gray-200 bg-gray-50 text-left">
                <th className="px-5 py-3 font-semibold text-gray-700">{t('user.username')}</th>
                <th className="px-5 py-3 font-semibold text-gray-700">{t('user.role')}</th>
                <th className="px-5 py-3 font-semibold text-gray-700">{t('user.createdAt')}</th>
                <th className="px-5 py-3 font-semibold text-gray-700">{t('user.actions')}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {users.map((user) => {
                const isSelf = user.id === currentUser?.id
                return (
                  <tr key={user.id} className="transition hover:bg-gray-50">
                    <td className="px-5 py-3">
                      <div className="flex items-center gap-2">
                        <span className="inline-flex h-7 w-7 items-center justify-center rounded-full bg-indigo-100 text-xs font-bold uppercase text-indigo-600">
                          {user.username.charAt(0)}
                        </span>
                        <span className="font-medium text-gray-900">{user.username}</span>
                        {isSelf && (
                          <span className="rounded bg-indigo-50 px-1.5 py-0.5 text-xs text-indigo-500">
                            {t('user.you')}
                          </span>
                        )}
                      </div>
                    </td>
                    <td className="px-5 py-3">
                      <select
                        value={user.role}
                        onChange={(e) => handleRoleChange(user, e.target.value as Role)}
                        disabled={isSelf || updateRoleMutation.isPending}
                        title={isSelf ? 'Cannot change your own role' : undefined}
                        className="rounded-lg border border-gray-300 px-2 py-1 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed"
                      >
                        {ROLES.map((r) => (
                          <option key={r} value={r}>
                            {t(`user.roles.${r}`)}
                          </option>
                        ))}
                      </select>
                    </td>
                    <td className="px-5 py-3 text-gray-500">
                      {new Date(user.createdAt).toLocaleDateString()}
                    </td>
                    <td className="px-5 py-3">
                      {!isSelf && (
                        <button
                          onClick={() => handleDelete(user)}
                          disabled={deleteMutation.isPending}
                          className="rounded-lg border border-red-200 px-3 py-1 text-xs font-medium text-red-600 transition hover:bg-red-50 disabled:opacity-50"
                        >
                          {t('common.delete')}
                        </button>
                      )}
                    </td>
                  </tr>
                )
              })}
              {users.length === 0 && (
                <tr>
                  <td colSpan={4} className="px-5 py-8 text-center text-gray-400">
                    {t('common.noData')}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
