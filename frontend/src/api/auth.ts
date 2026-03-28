import { api } from './client'
import type { User, AuthResponse } from '../types/formula'

export function login(username: string, password: string): Promise<AuthResponse> {
  return api.post<AuthResponse>('/auth/login', { username, password })
}

export function register(username: string, password: string): Promise<AuthResponse> {
  return api.post<AuthResponse>('/auth/register', { username, password })
}

export function getMe(): Promise<User> {
  return api.get<User>('/auth/me')
}
