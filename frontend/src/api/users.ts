import { api } from './client'
import type { User, Role } from '../types/formula'

export interface ListUsersResponse {
  users: User[]
}

export function listUsers(): Promise<ListUsersResponse> {
  return api.get<ListUsersResponse>('/users')
}

export function updateUserRole(id: string, role: Role): Promise<User> {
  return api.patch<User>(`/users/${id}/role`, { role })
}

export function deleteUser(id: string): Promise<void> {
  return api.delete<void>(`/users/${id}`)
}
