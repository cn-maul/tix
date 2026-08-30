import { useState, useEffect } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { generateAPIKey, getAPIKey, getSettings, updateSettings } from '@/api/settings'

const DEFAULT_SITE_NAME = 'tix 工单系统'

export default function General() {
  const queryClient = useQueryClient()
  const [name, setName] = useState('')
  const [saved, setSaved] = useState('')
  const [generating, setGenerating] = useState(false)

  const { data: settings } = useQuery({
    queryKey: ['settings'],
    queryFn: getSettings,
  })

  const { data: apiKey } = useQuery({
    queryKey: ['api-key'],
    queryFn: getAPIKey,
  })

  useEffect(() => {
    if (settings) {
      const v = settings.site_name || DEFAULT_SITE_NAME
      setName(v)
      setSaved(v)
    }
  }, [settings])

  const regenerateKey = async () => {
    if (apiKey && !window.confirm('重新生成后旧 Key 立即失效，已接入的外部工具将无法继续访问，确定继续？')) {
      return
    }
    setGenerating(true)
    try {
      const key = await generateAPIKey()
      queryClient.setQueryData(['api-key'], key)
      toast.success('已生成 API Key')
    } catch {
      toast.error('生成失败')
    } finally {
      setGenerating(false)
    }
  }

  const copyKey = async () => {
    if (!apiKey) return
    try {
      await navigator.clipboard.writeText(apiKey)
      toast.success('已复制到剪贴板')
    } catch {
      toast.error('复制失败，请手动选择复制')
    }
  }

  const save = async () => {
    const v = name.trim() || DEFAULT_SITE_NAME
    try {
      await updateSettings({ site_name: v })
      setName(v)
      setSaved(v)
      queryClient.invalidateQueries({ queryKey: ['settings'] })
      toast.success('已保存')
    } catch {
      toast.error('保存失败')
    }
  }

  return (
    <div className="space-y-4">
      <h1 className="text-xl font-semibold">通用设置</h1>
      <Card>
        <CardHeader>
          <CardTitle>网站命名</CardTitle>
          <CardDescription>自定义左上角和登录页显示的网站名称</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="site-name">网站名称</Label>
            <Input
              id="site-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={DEFAULT_SITE_NAME}
              maxLength={20}
            />
          </div>
          <div className="flex items-center gap-3">
            <Button onClick={save} disabled={name.trim() === saved}>
              保存
            </Button>
            {saved !== DEFAULT_SITE_NAME && (
              <Button
                variant="ghost"
                onClick={async () => {
                  try {
                    await updateSettings({ site_name: DEFAULT_SITE_NAME })
                    setName(DEFAULT_SITE_NAME)
                    setSaved(DEFAULT_SITE_NAME)
                    queryClient.invalidateQueries({ queryKey: ['settings'] })
                  } catch {
                    toast.error('恢复失败')
                  }
                }}
              >
                恢复默认
              </Button>
            )}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>集成 API Key</CardTitle>
          <CardDescription>
            供外部工具（如时间规划助手）通过 X-API-Key 请求头免登录读取工单。
            持 Key 者拥有与操作员相同的接口权限，请妥善保管；重新生成后旧 Key 立即失效。
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="flex items-center gap-2">
            <code className="min-w-0 flex-1 truncate rounded-md bg-muted px-3 py-2 font-mono text-sm">
              {apiKey || '尚未生成'}
            </code>
            {apiKey && (
              <Button variant="outline" onClick={copyKey}>
                复制
              </Button>
            )}
            <Button variant={apiKey ? 'ghost' : 'default'} onClick={regenerateKey} disabled={generating}>
              {apiKey ? '重新生成' : '生成 Key'}
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
