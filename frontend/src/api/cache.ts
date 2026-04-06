import { api } from './client'

export interface CacheStats {
  size: number
  maxSize: number
}

export function getCacheStats(): Promise<CacheStats> {
  return api.get<CacheStats>('/cache')
}

export function clearCache(): Promise<void> {
  return api.delete('/cache')
}
