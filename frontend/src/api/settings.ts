import { api } from './client'

export interface SettingsData {
  maxConcurrentCalcs: number
}

export function getSettings(): Promise<SettingsData> {
  return api.get<SettingsData>('/settings')
}

export function updateSettings(data: Partial<SettingsData>): Promise<SettingsData> {
  return api.put<SettingsData>('/settings', data)
}
