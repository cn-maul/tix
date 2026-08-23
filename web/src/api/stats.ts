import client from './client'

export interface Stats {
  pending: number
  done: number
  today_new: number
  by_cat: { category: string; count: number }[]
  by_day: { category: string; count: number }[]
  by_day_cat: { day: string; category: string; count: number }[]
  month_cat: { category: string; count: number }[]
}

export async function fetchStats(): Promise<Stats> {
  const r = await client.get<{ data: Stats }>('/stats')
  return r.data.data
}
