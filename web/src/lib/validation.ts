import { useCallback, useRef, useState } from 'react'
import type { FormEvent } from 'react'

// 轻量表单校验：替代此前的 zod + react-hook-form 组合，
// 校验规则与后端 api.go 保持一致，错误文案不变。

// ---------- 表单数据类型 ----------

export interface LoginFormValues {
  username: string
  password: string
}

export interface CategoryFormValues {
  name: string
  color: string
  sort: number
  enabled: boolean
}

export type Errors<F> = Partial<Record<keyof F, string>>

// ---------- 校验规则 ----------

export function validateLogin(v: LoginFormValues): Errors<LoginFormValues> {
  const e: Errors<LoginFormValues> = {}
  if (!v.username) e.username = '请输入用户名'
  if (!v.password) e.password = '请输入密码'
  return e
}

// 工单表单（游客提交页与管理端新建/编辑共用）：姓名与手机号为两个独立字段，
// 分别落库到 tickets.creator 与 tickets.phone；手机号是游客进度查询的凭据。
export interface TicketInputValues {
  category: string
  name: string
  phone: string
  content: string
}

export function validateTicketInput(v: TicketInputValues): Errors<TicketInputValues> {
  const e: Errors<TicketInputValues> = {}
  if (!v.category) e.category = '请选择分类'
  if (!v.name.trim()) e.name = '请输入姓名'
  else if (v.name.trim().length > 20) e.name = '姓名最多 20 个字符'
  const phone = v.phone.trim()
  if (!phone) e.phone = '请输入手机号'
  else if (!/^1[3-9]\d{9}$/.test(phone)) e.phone = '请输入正确的 11 位手机号'
  if (!v.content) e.content = '请输入内容'
  else if (v.content.length > 50) e.content = '最多 50 字'
  return e
}

export function validateCategory(v: CategoryFormValues): Errors<CategoryFormValues> {
  const e: Errors<CategoryFormValues> = {}
  if (!v.name) e.name = '请输入分类名'
  return e
}

export interface PasswordChangeValues {
  old_password: string
  new_password: string
  confirm: string
}

export function validatePasswordChange(v: PasswordChangeValues): Errors<PasswordChangeValues> {
  const e: Errors<PasswordChangeValues> = {}
  if (!v.old_password) e.old_password = '请输入旧密码'
  if (!v.new_password) e.new_password = '请输入新密码'
  else if (v.new_password.length < 6) e.new_password = '新密码长度须至少6位'
  if (!v.confirm) e.confirm = '请再次输入新密码'
  else if (v.confirm !== v.new_password) e.confirm = '两次输入不一致'
  return e
}

export interface UserCreateValues {
  username: string
  password: string
  display_name: string
}

export function validateUserCreate(v: UserCreateValues): Errors<UserCreateValues> {
  const e: Errors<UserCreateValues> = {}
  if (!v.username) e.username = '请输入用户名'
  else if (!/^[a-zA-Z0-9_]{3,32}$/.test(v.username)) e.username = '用户名须为 3-32 位字母、数字或下划线'
  if (!v.password) e.password = '请输入密码'
  else if (v.password.length < 6) e.password = '密码长度须至少6位'
  if (!v.display_name) e.display_name = '请输入显示名称'
  else if (v.display_name.length > 32) e.display_name = '最多 32 字符'
  return e
}

export interface UserEditValues {
  display_name: string
  password: string // 留空则不修改
}

export function validateUserEdit(v: UserEditValues): Errors<UserEditValues> {
  const e: Errors<UserEditValues> = {}
  if (!v.display_name) e.display_name = '请输入显示名称'
  else if (v.display_name.length > 32) e.display_name = '最多 32 字符'
  if (v.password && v.password.length < 6) e.password = '新密码长度须至少6位'
  return e
}

// ---------- 极简表单状态 Hook（替代 react-hook-form） ----------

// 提交时整体校验；出错字段在再次输入时清除错误。
// set/getValues/reset/submit 引用稳定，可安全加入 effect 依赖。
export function useFormState<F extends object>(initial: F, validate: (v: F) => Errors<F>) {
  const [values, setValues] = useState<F>(initial)
  const [errors, setErrors] = useState<Errors<F>>({})
  // 始终持有最新值/校验器，保证回调引用稳定且不读旧值
  const valuesRef = useRef(values)
  valuesRef.current = values
  const validateRef = useRef(validate)
  validateRef.current = validate
  const initialRef = useRef(initial)

  const set = useCallback(<K extends keyof F>(key: K, value: F[K]) => {
    setValues((prev) => ({ ...prev, [key]: value }))
    setErrors((prev) => (prev[key] ? { ...prev, [key]: undefined } : prev))
  }, [])

  const getValues = useCallback(() => valuesRef.current, [])

  const reset = useCallback((next?: F) => {
    setValues(next === undefined ? initialRef.current : next)
    setErrors({})
  }, [])

  // 包装 form onSubmit：阻止默认提交 → 整体校验 → 全部通过才调用 onValid
  const submit = useCallback(
    (onValid: (v: F) => void | Promise<void>) =>
      (e: FormEvent<HTMLFormElement>) => {
        e.preventDefault()
        const errs = validateRef.current(valuesRef.current)
        setErrors(errs)
        if (!Object.values(errs).some(Boolean)) void onValid(valuesRef.current)
      },
    [],
  )

  return { values, errors, set, getValues, reset, submit }
}
