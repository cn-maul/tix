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
  type NotifyConfig,
  type NotifyConfigUpdate,
} from '@/api/notifications'

export default function Notifications() {
  const queryClient = useQueryClient()
  // PushPlus
  const [ppEnabled, setPpEnabled] = useState(false)
  const [ppToken, setPpToken] = useState('')
  const [ppTopic, setPpTopic] = useState('')
  // Server酱
  const [scEnabled, setScEnabled] = useState(false)
  const [scKey, setScKey] = useState('')

  const { data: cfg, isLoading } = useQuery({
    queryKey: ['notify-config'],
    queryFn: getNotifyConfig,
  })

  useEffect(() => {
    if (cfg) {
      setPpEnabled(cfg.pushplus.enabled === 1)
      setPpToken('')
      setPpTopic(cfg.pushplus.topic || '')
      setScEnabled(cfg.serverchan.enabled === 1)
      setScKey('')
    }
  }, [cfg])

  // 是否有未保存的修改（任一渠道）
  const saved = cfg as NotifyConfig | undefined
  const ppDirty =
    !!saved &&
    (ppEnabled !== (saved.pushplus.enabled === 1) ||
      ppToken.trim() !== '' ||
      ppTopic.trim() !== saved.pushplus.topic)
  const scDirty =
    !!saved && (scEnabled !== (saved.serverchan.enabled === 1) || scKey.trim() !== '')
  const dirty = ppDirty || scDirty

  const save = async () => {
    if (!saved) return
    try {
      const body: NotifyConfigUpdate = {}
      if (ppDirty) {
        body.pushplus = {
          enabled: ppEnabled ? 1 : 0,
          topic: ppTopic.trim(),
          ...(ppToken.trim() ? { token: ppToken.trim() } : {}),
        }
      }
      if (scDirty) {
        body.serverchan = {
          enabled: scEnabled ? 1 : 0,
          ...(scKey.trim() ? { sendkey: scKey.trim() } : {}),
        }
      }
      await updateNotifyConfig(body)
      setPpToken('')
      setScKey('')
      queryClient.invalidateQueries({ queryKey: ['notify-config'] })
      toast.success('已保存')
    } catch (e: any) {
      toast.error(e?.message || '保存失败')
    }
  }

  const sendTest = async () => {
    try {
      const results = await testNotify()
      if (results.length > 0 && results.every((r) => r.ok)) {
        toast.success(`测试消息已发送至 ${results.length} 个已启用渠道`)
      } else {
        const failed = results.filter((r) => !r.ok)
        toast.error(failed.map((r) => `${r.channel}：${r.error || '发送失败'}`).join('；') || '发送失败')
      }
    } catch (e: any) {
      toast.error(e?.message || '发送失败')
    }
  }

  const anyChannelReady =
    (ppEnabled && (saved?.pushplus.token_set || ppToken.trim() !== '')) ||
    (scEnabled && (saved?.serverchan.sendkey_set || scKey.trim() !== ''))

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-xl font-semibold">消息推送</h1>
        <div className="flex items-center gap-3">
          <Button onClick={save} disabled={!dirty}>
            保存全部
          </Button>
          <Button variant="outline" onClick={sendTest} disabled={!anyChannelReady || isLoading}>
            <SendHorizonal className="size-4" />
            发送测试消息
          </Button>
        </div>
      </div>
      {!anyChannelReady && (
        <p className="text-xs text-muted-foreground">
          启用至少一个渠道并完成配置后，可发送测试消息验证通道。
        </p>
      )}

      {/* PushPlus */}
      <Card>
        <CardHeader>
          <CardTitle>推送渠道 · PushPlus</CardTitle>
          <CardDescription>
            通过{' '}
            <a
              href="https://www.pushplus.plus/"
              target="_blank"
              rel="noreferrer"
              className="text-primary underline underline-offset-2"
            >
              pushplus.plus
            </a>{' '}
            把通知推送到微信。微信扫码登录后即可免费获取 Token。
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-between rounded-lg border p-3">
            <div className="space-y-0.5">
              <Label htmlFor="pp-enabled">启用渠道</Label>
              <p className="text-xs text-muted-foreground">关闭后该渠道不发送任何消息</p>
            </div>
            <Switch id="pp-enabled" checked={ppEnabled} onCheckedChange={setPpEnabled} disabled={isLoading} />
          </div>

          <div className="space-y-2">
            <Label htmlFor="pp-token">PushPlus Token</Label>
            <Input
              id="pp-token"
              type="password"
              value={ppToken}
              onChange={(e) => setPpToken(e.target.value)}
              placeholder={
                saved?.pushplus.token_set
                  ? `已保存（${saved.pushplus.token_masked}），留空则不修改`
                  : '在 pushplus.plus 获取'
              }
              autoComplete="new-password"
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="pp-topic">群组编码（可选）</Label>
            <Input
              id="pp-topic"
              value={ppTopic}
              onChange={(e) => setPpTopic(e.target.value)}
              placeholder="留空发送给 Token 本人；填写后推送给该群组全部成员"
              maxLength={64}
            />
            <p className="text-xs text-muted-foreground">群组需已在 PushPlus 完成成员配对</p>
          </div>
        </CardContent>
      </Card>

      {/* Server酱 */}
      <Card>
        <CardHeader>
          <CardTitle>推送渠道 · Server酱</CardTitle>
          <CardDescription>
            通过{' '}
            <a
              href="https://sct.ftqq.com/"
              target="_blank"
              rel="noreferrer"
              className="text-primary underline underline-offset-2"
            >
              sct.ftqq.com
            </a>{' '}
            推送消息到微信 / 企业微信。在官网微信扫码登录后复制 SendKey 填入。
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-between rounded-lg border p-3">
            <div className="space-y-0.5">
              <Label htmlFor="sc-enabled">启用渠道</Label>
              <p className="text-xs text-muted-foreground">关闭后该渠道不发送任何消息</p>
            </div>
            <Switch id="sc-enabled" checked={scEnabled} onCheckedChange={setScEnabled} disabled={isLoading} />
          </div>

          <div className="space-y-2">
            <Label htmlFor="sc-key">Server酱 SendKey</Label>
            <Input
              id="sc-key"
              type="password"
              value={scKey}
              onChange={(e) => setScKey(e.target.value)}
              placeholder={
                saved?.serverchan.sendkey_set
                  ? `已保存（${saved.serverchan.sendkey_masked}），留空则不修改`
                  : '形如 SCTxxxx 的 SendKey'
              }
              autoComplete="new-password"
            />
            <p className="text-xs text-muted-foreground">
              SendKey 等同于账号凭据，请勿泄露；服务端仅以脱敏形式回显
            </p>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
