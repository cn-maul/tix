import client from './client'

export interface Ticket {
  id: number
  category: string
  content: string
  creator: string
  status: 0 | 1
  created_at: string
  updated_at: string
}

export interface Comment {
  id: number
  ticket_id: number
  author: string
  content: string
  created_at: string
}

export interface ListParams {
  status?: 0 | 1 | ''
  category?: string
  keyword?: string
  page?: number
  size?: number
  order?: 'asc' | 'desc'
}

export interface ListResult {
  items: Ticket[]
  total: number
  page: number
  size: number
}

export interface CreatePayload {
  category: string
  content: string
  creator: string
}

export async function fetchList(p: ListParams = {}): Promise<ListResult> {
  const params: Record<string, any> = {}
  if (Object.prototype.hasOwnProperty.call(p, 'status')) params.status = p.status
  if (p.category) params.category = p.category
  if (p.keyword) params.keyword = p.keyword
  if (p.page) params.page = p.page
  if (p.size) params.size = p.size
  if (p.order) params.order = p.order
  const r = await client.get<ListResult>('/tickets', { params })
  return r.data
}

export async function fetchOne(id: number): Promise<Ticket> {
  const r = await client.get<{ data: Ticket }>(`/tickets/${id}`)
  return r.data.data
}

export async function createTicket(p: CreatePayload): Promise<Ticket> {
  const r = await client.post<{ data: Ticket }>('/tickets', p)
  return r.data.data
}

// 公开提交：走 /api/submit（免密码）
export async function submitTicket(p: CreatePayload): Promise<Ticket> {
  const r = await client.post<{ data: Ticket }>('/submit', p)
  return r.data.data
}

export async function updateTicket(id: number, p: CreatePayload): Promise<Ticket> {
  const r = await client.put<{ data: Ticket }>(`/tickets/${id}`, p)
  return r.data.data
}

export async function markDone(id: number, note?: string, author?: string): Promise<Ticket> {
  const r = await client.post<{ data: Ticket }>(`/tickets/${id}/done`, {
    note: note || undefined,
    author: author || undefined,
  })
  return r.data.data
}

export async function deleteTicket(id: number): Promise<void> {
  await client.post(`/tickets/${id}/delete`)
}

// 备注 / 处理记录
export async function fetchComments(ticketId: number): Promise<Comment[]> {
  const r = await client.get<{ items: Comment[] }>(`/tickets/${ticketId}/comments`)
  return r.data.items
}

export async function addComment(ticketId: number, author: string, content: string): Promise<Comment> {
  const r = await client.post<{ data: Comment }>(`/tickets/${ticketId}/comments`, { author, content })
  return r.data.data
}

// 工单编号：T-YYYYMMDD-NNNN（如 T-20260818-0001）
export function ticketNumber(t: Ticket): string {
  const date = t.created_at.split(' ')[0].split('-').join('')
  return `T-${date}-${String(t.id).padStart(4, '0')}`
}
