import client from './client'

export interface User {
  id: number
  username: string
  display_name: string
  role: 'admin' | 'operator'
  created_at: string
}

export async function fetchAuthStatus(): Promise<{ ok: boolean; user?: User }> {
  try {
    const r = await client.get('/auth/status')
    return r.data?.data ?? { ok: false }
  } catch {
    return { ok: false }
  }
}

export async function login(username: string, password: string): Promise<{ user: User }> {
  const r = await client.post('/login', { username, password })
  return r.data?.data
}

export async function logout(): Promise<void> {
  await client.post('/logout')
}

export async function fetchUsers(): Promise<User[]> {
  const r = await client.get('/users')
  return r.data?.data ?? []
}

export async function createUser(data: {
  username: string
  password: string
  display_name: string
  role: string
}): Promise<{ id: number }> {
  const r = await client.post('/users', data)
  return r.data?.data
}

export async function updateUser(
  id: number,
  data: { display_name: string; role: string; password?: string },
): Promise<void> {
  await client.put(`/users/${id}`, data)
}

export async function deleteUser(id: number): Promise<void> {
  await client.delete(`/users/${id}`)
}
