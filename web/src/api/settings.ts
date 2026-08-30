import client from './client'

export async function getSettings(): Promise<Record<string, string>> {
  const r = await client.get('/settings')
  return r.data?.data ?? {}
}

export async function updateSettings(settings: Record<string, string>): Promise<void> {
  await client.put('/settings', settings)
}

/** 查看外部集成 API Key（管理员） */
export async function getAPIKey(): Promise<string> {
  const r = await client.get('/settings/api-key')
  return r.data?.data?.api_key ?? ''
}

/** 生成（或轮换）外部集成 API Key（管理员），旧 Key 立即失效 */
export async function generateAPIKey(): Promise<string> {
  const r = await client.post('/settings/api-key/generate')
  return r.data?.data?.api_key ?? ''
}
