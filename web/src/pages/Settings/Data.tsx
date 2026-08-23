import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Download } from 'lucide-react'
import client from '@/api/client'
import { fetchCategories } from '../../api/categories'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

export default function Data() {
  const [status, setStatus] = useState('')
  const [category, setCategory] = useState('')
  const [loading, setLoading] = useState(false)

  const { data: categories } = useQuery({ queryKey: ['categories'], queryFn: fetchCategories })

  const exportCSV = async () => {
    setLoading(true)
    try {
      const params = new URLSearchParams()
      if (status !== '') params.set('status', status)
      if (category) params.set('category', category)
      // 走 axios 携带会话下载，服务端错误可被捕获并提示
      // （直接 <a download> 导航时 401 等错误会被浏览器存成 .csv 文件）
      const r = await client.get('/export/csv', { params, responseType: 'blob' })
      const url = URL.createObjectURL(r.data)
      const a = document.createElement('a')
      a.href = url
      a.download = `tix-${new Date().toISOString().slice(0, 10)}.csv`
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      URL.revokeObjectURL(url)
      toast.success('导出成功')
    } catch {
      toast.error('导出失败，请确认登录状态后重试')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="space-y-4">
      <h1 className="text-xl font-semibold">数据与备份</h1>
      <p className="text-sm text-muted-foreground">
        数据存储在 SQLite 文件 <code className="rounded bg-muted px-1.5 py-0.5 font-mono">tix.db</code> 中，建议定期备份该文件。下方可按条件导出 CSV 工单清单。
      </p>

      <Card>
        <CardHeader>
          <CardTitle>导出 CSV</CardTitle>
          <CardDescription>按状态与分类筛选后导出</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-wrap items-center gap-2">
          <Select value={status || 'all'} onValueChange={(v) => setStatus(v === 'all' ? '' : v)}>
            <SelectTrigger className="w-36">
              <SelectValue placeholder="全部状态" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部状态</SelectItem>
              <SelectItem value="0">待处理</SelectItem>
              <SelectItem value="1">已处理</SelectItem>
            </SelectContent>
          </Select>

          <Select value={category || 'all'} onValueChange={(v) => setCategory(v === 'all' ? '' : v)}>
            <SelectTrigger className="w-36">
              <SelectValue placeholder="全部分类" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部分类</SelectItem>
              {(categories ?? []).map((c) => (
                <SelectItem key={c.id} value={c.name}>
                  {c.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <Button onClick={exportCSV} loading={loading}>
            <Download /> 导出 CSV
          </Button>
        </CardContent>
      </Card>
    </div>
  )
}