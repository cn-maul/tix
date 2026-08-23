import client from './client'

export interface Category {
  id: number
  name: string
  color: string
  sort: number
  enabled: number // 1 启用 0 停用
}

export async function fetchCategories(): Promise<Category[]> {
  const r = await client.get<{ items: Category[] }>('/categories')
  return r.data.items
}

// 公开接口：提交页可用的已启用分类名（无需登录）
export async function fetchSubmitCategories(): Promise<string[]> {
  const r = await client.get<{ items: string[] }>('/submit/categories')
  return r.data.items
}

export async function createCategory(p: { name: string; color?: string; sort?: number }): Promise<Category> {
  const r = await client.post<{ data: Category }>('/categories', p)
  return r.data.data
}

export async function updateCategory(id: number, p: { name?: string; color?: string; sort?: number; enabled?: number }): Promise<Category> {
  const r = await client.put<{ data: Category }>(`/categories/${id}`, p)
  return r.data.data
}

export async function deleteCategory(id: number): Promise<void> {
  await client.delete(`/categories/${id}`)
}
