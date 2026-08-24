import client from './client'

export interface Ticket {
  id: number
  category: string
  content: string
  creator: string // 发起人姓名
  phone: string // 发起人手机号（游客进度查询凭据）
  status: 0 | 1
  created_at: string
  updated_at: string
  assignee: string
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
  from?: string // YYYY-MM-DD，含边界日
  to?: string
  assignee?: string // 精确用户名或 "me"（当前登录用户）
  unassigned?: boolean
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
  name: string // 发起人姓名
  phone: string // 发起人手机号
}

export async function fetchList(p: ListParams = {}): Promise<ListResult> {
  const params: Record<string, any> = {}
  if (Object.prototype.hasOwnProperty.call(p, 'status')) params.status = p.status
  if (p.category) params.category = p.category
  if (p.keyword) params.keyword = p.keyword
  if (p.from) params.from = p.from
  if (p.to) params.to = p.to
  if (p.assignee) params.assignee = p.assignee
  if (p.unassigned) params.unassigned = '1'
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

// 指派 / 取消负责人（空串取消）
export async function assignTicket(id: number, assignee: string): Promise<Ticket> {
  const r = await client.post<{ data: Ticket }>(`/tickets/${id}/assign`, { assignee })
  return r.data.data
}

// 批量标记已处理，返回实际更新条数
export async function batchMarkDone(ids: number[]): Promise<number> {
  const r = await client.post<{ data: { updated: number } }>('/tickets/batch-done', { ids })
  return r.data.data.updated
}

// 批量删除（连带备注），返回实际删除条数
export async function batchDeleteTickets(ids: number[]): Promise<number> {
  const r = await client.post<{ data: { deleted: number } }>('/tickets/batch-delete', { ids })
  return r.data.data.deleted
}

// 按筛选条件导出 CSV 并触发下载；返回是否因超上限被截断
export async function exportCsv(p: ListParams = {}): Promise<boolean> {
  const params: Record<string, any> = {}
  if (Object.prototype.hasOwnProperty.call(p, 'status')) params.status = p.status
  if (p.category) params.category = p.category
  if (p.keyword) params.keyword = p.keyword
  if (p.from) params.from = p.from
  if (p.to) params.to = p.to
  if (p.assignee) params.assignee = p.assignee
  if (p.unassigned) params.unassigned = '1'
  // 走 axios 携带会话下载，服务端错误可被捕获并提示
  // （直接 <a download> 导航时 401 等错误会被浏览器存成 .csv 文件）
  const r = await client.get('/export/csv', { params, responseType: 'blob' })
  const url = URL.createObjectURL(r.data)
  const a = document.createElement('a')
  a.href = url
  a.download = `tix-${new Date().toISOString().slice(0, 10)}.csv`
  a.click()
  URL.revokeObjectURL(url)
  return r.headers['x-tix-truncated'] === '1'
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

// --------------------------------------------------------------------
// 游客进度查询（公开接口；凭提交时填写的姓名/联系方式精确匹配）
// --------------------------------------------------------------------

// 游客凭手机号查询自己提交的工单列表（最近 50 条，新的在前）
export async function fetchMyTickets(phone: string): Promise<Ticket[]> {
  const r = await client.get<{ items: Ticket[] }>('/my/tickets', { params: { phone } })
  return r.data?.items ?? []
}

export interface GuestTicketDetail {
  ticket: Ticket
  comments: Comment[]
}

// 游客查询单条工单详情（含处理记录）；手机号不匹配时服务端返回 404
export async function fetchMyTicketDetail(id: number, phone: string): Promise<GuestTicketDetail> {
  const r = await client.get<{ data: GuestTicketDetail }>(`/my/tickets/${id}`, {
    params: { phone },
  })
  return r.data.data
}
