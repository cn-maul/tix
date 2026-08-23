import { useState, useEffect } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { getSettings, updateSettings } from '@/api/settings'

const DEFAULT_SITE_NAME = 'tix 工单系统'

export default function General() {
  const queryClient = useQueryClient()
  const [name, setName] = useState('')
  const [saved, setSaved] = useState('')

  const { data: settings } = useQuery({
    queryKey: ['settings'],
    queryFn: getSettings,
  })

  useEffect(() => {
    if (settings) {
      const v = settings.site_name || DEFAULT_SITE_NAME
      setName(v)
      setSaved(v)
    }
  }, [settings])

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
    </div>
  )
}
