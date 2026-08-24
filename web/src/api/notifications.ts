import client from './client'

// 推送配置（分渠道；密钥仅返回脱敏形式）
export interface PushPlusConfig {
  enabled: number
  token_set: boolean
  token_masked: string
  topic: string
}

export interface ServerChanConfig {
  enabled: number
  sendkey_set: boolean
  sendkey_masked: string
}

export interface NotifyConfig {
  pushplus: PushPlusConfig
  serverchan: ServerChanConfig
}

// 单渠道发送结果
export interface NotifyResult {
  channel: string
  ok: boolean
  error?: string
}

export async function getNotifyConfig(): Promise<NotifyConfig> {
  const r = await client.get<{ data: NotifyConfig }>('/notify/config')
  return (
    r.data?.data ?? {
      pushplus: { enabled: 0, token_set: false, token_masked: '', topic: '' },
      serverchan: { enabled: 0, sendkey_set: false, sendkey_masked: '' },
    }
  )
}

// 各字段出现即生效：密钥传空串清除，不传保持不变
export interface NotifyConfigUpdate {
  pushplus?: { enabled?: number; token?: string; topic?: string }
  serverchan?: { enabled?: number; sendkey?: string }
}

export async function updateNotifyConfig(cfg: NotifyConfigUpdate): Promise<NotifyConfig> {
  const r = await client.put<{ data: NotifyConfig }>('/notify/config', cfg)
  return r.data?.data
}

export async function testNotify(): Promise<NotifyResult[]> {
  const r = await client.post<{ data: { results: NotifyResult[] } }>('/notify/test')
  return r.data?.data?.results ?? []
}
