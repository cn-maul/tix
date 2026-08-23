import client from './client'

export async function getSettings(): Promise<Record<string, string>> {
  const r = await client.get('/settings')
  return r.data?.data ?? {}
}

export async function updateSettings(settings: Record<string, string>): Promise<void> {
  await client.put('/settings', settings)
}
