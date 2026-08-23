import { z } from 'zod'

export const ticketSchema = z.object({
  category: z.string().min(1, '请选择分类'),
  creator: z.string().min(1, '请输入发起人').max(16, '最多 16 个字符'),
  content: z.string().min(1, '请输入内容').max(50, '最多 50 字'),
})
export type TicketFormValues = z.infer<typeof ticketSchema>

export const loginSchema = z.object({
  username: z.string().min(1, '请输入用户名'),
  password: z
    .string()
    .min(1, '请输入密码'),
})
export type LoginFormValues = z.infer<typeof loginSchema>

export const categorySchema = z.object({
  name: z.string().min(1, '请输入分类名'),
  color: z.string(),
  sort: z.coerce.number().int(),
  enabled: z.boolean(),
})
export type CategoryFormValues = z.infer<typeof categorySchema>