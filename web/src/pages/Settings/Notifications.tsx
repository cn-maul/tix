import { useState, useEffect } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { SendHorizonal } from 'lucide-react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  getNotifyConfig,
  updateNotifyConfig,
  testNotify,
  type NotifyResult,
} from '@/api/notifications'

export default function Notifications() {
  const queryClient = useQueryClient()
  const [enabled, setEnabled] = useState(false)
  const [token, setToken] = useState('')
  const [topic, setTopic] = useState('')
  const [savedTopic, setSavedTopic] = useState('')

  const { data: cfg, isLoading } = useQuery({
    queryKey: ['notify-config'],
    queryFn: getNotifyConfig,
  })

  useEffect(() => {
    if (cfg) {
      setEnabled(cfg.enabled === 1)
      setToken('')
      setTopic(cfg.topic || '')
      setSavedTopic(cfg.topic || '')
    }
  }, [cfg])

  // 是否有未保存的修改：开关、群组编码，或输入了新 Token
  const savedEnabled = cfg?.enabled === 1
  const dirty =
    !!cfg && (enabled !== savedEnabled || token.trim() !== '' || topic.trim() !== savedTopic)

  const save = async () => {
    try {
      await updateNotifyConfig({
        enabled: enabled ? 1 : 0,
        ...(token.trim() !== '' ? { token: token.trim() } : {}),
        topic: topic.trim(),
      })
      setToken('')
      setSavedTopic(topic.trim())
      queryClient.invalidateQueries({ queryKey: ['notify-config'] })
      toast.success('已保存')
    } catch (e: any) {
      toast.error(e.message || '保存失败')
    }
  }

  const sendTest = async () => {
    try {
      const results: NotifyResult[] = await testNotify()
      if (results.length > 0 && results.every((r) => r.ok)) {
        toast.success('测试消息已发送，请到微信查收')
      } else {
        const failed = results.filter((r) => !r.ok)
        toast.error(failed.map((r) => `${r.channel}：${r.error || '发送失败'}`).join('；') || '发送失败')
      }
    } catch (e: any) {
      toast.error(e.message || '发送失败')
    }
  }

  const canTest = enabled && cfg?.token_set

  return (
    <div className="space-y-4">
      <h1 className="text-xl font-semibold">消息推送</h1>
      <Card>
        <CardHeader>
          <CardTitle>推送渠道 · PushPlus</CardTitle>
          <CardDescription>
            通过 PushPlus（pushplus.plus）把通知推送到微信。在
            <a
              href="https://www.pushplus.plus/"
              target="_blank"
              rel="noreferrer"
              className="mx-1 text-primary underline underline-offset-2"
            >
              pushplus.plus
            </a>
            微信扫码登录后即可免费获取 Token。
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-between rounded-lg border p-3">
            <div className="space-y-0.5">
              <Label htmlFor="notify-enabled">启用推送</Label>
              <p className="text-xs text-muted-foreground">关闭后将不发送任何推送（当前支持手动发送测试消息验证通道）</p>
            </div>
            <Switch id="notify-enabled" checked={enabled} onCheckedChange={setEnabled} disabled={isLoading} />
          </div>

          <div className="space-y-2">
            <Label htmlFor="notify-token">PushPlus Token</Label>
            <Input
              id="notify-token"
              type="password"
              value={token}
              onChange={(e) => setToken(e.target.value)}
              placeholder={cfg?.token_set ? `已保存（${cfg.token_masked}），留空则不修改` : '在 pushplus.plus 获取'}
              autoComplete="new-password"
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="notify-topic">群组编码（可选）</Label>
            <Input
              id="notify-topic"
              value={topic}
              onChange={(e) => setTopic(e.target.value)}
              placeholder="留空发送给 Token 本人；填写后推送给该群组全部成员"
              maxLength={64}
            />
            <p className="text-xs text-muted-foreground">群组需已在 PushPlus 完成成员配对</p>
          </div>

          <div className="flex items-center gap-3">
            <Button onClick={save} disabled={!dirty}>
              保存
            </Button>
            <Button variant="outline" onClick={sendTest} disabled={!canTest || isLoading}>
              <SendHorizonal className="size-4" />
              发送测试消息
            </Button>
          </div>
          {!canTest && (
            <p className="text-xs text-muted-foreground">启用推送并保存 Token 后可发送测试消息验证通道。</p>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
