import client from './client'

// 推送配置（Token 仅返回脱敏形式）
export interface NotifyConfig {
  enabled: number
  token_set: boolean
  token_masked: string
  topic: string
}

// 单渠道发送结果
export interface NotifyResult {
  channel: string
  ok: boolean
  error?: string
}

export async function getNotifyConfig(): Promise<NotifyConfig> {
  const r = await client.get('/notify/config')
  return r.data?.data ?? { enabled: 0, token_set: false, token_masked: '', topic: '' }
}

// token 不传表示保持不变；传空串清除
export async function updateNotifyConfig(
  cfg: { enabled?: number; token?: string; topic?: string },
): Promise<NotifyConfig> {
  const r = await client.put('/notify/config', cfg)
  return r.data?.data
}

export async function testNotify(): Promise<NotifyResult[]> {
  const r = await client.post('/notify/test')
  return r.data?.data?.results ?? []
}
