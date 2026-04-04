import { api } from './client'
import type { Category } from '../types/formula'

export interface ListCategoriesResponse {
  categories: Category[]
}

export interface CreateCategoryData {
  slug: string
  name: string
  description: string
  color: string
  sortOrder: number
}

export interface UpdateCategoryData {
  name?: string
  description?: string
  color?: string
  sortOrder?: number
}

export function listCategories(): Promise<ListCategoriesResponse> {
  return api.get<ListCategoriesResponse>('/categories')
}

export function createCategory(data: CreateCategoryData): Promise<Category> {
  return api.post<Category>('/categories', data)
}

export function updateCategory(id: string, data: UpdateCategoryData): Promise<Category> {
  return api.put<Category>(`/categories/${id}`, data)
}

export function deleteCategory(id: string): Promise<void> {
  return api.delete<void>(`/categories/${id}`)
}
