import { CheckCircle2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'

export default function SubmitSuccess({ phone, onTrack, onSubmitAnother }: {
  phone: string
  onTrack: () => void
  onSubmitAnother: () => void
}) {
  return <div className="flex min-h-screen items-center justify-center bg-muted/40 p-4"><Card className="w-full max-w-sm shadow-lg"><CardContent className="flex flex-col items-center gap-3 pt-8 text-center"><CheckCircle2 className="size-12 text-emerald-500" /><h2 className="text-lg font-semibold">提交成功</h2><p className="text-sm text-muted-foreground">工单已提交，我们会尽快处理。<br />可随时凭手机号 <span className="font-medium text-foreground">{phone}</span> 查询进度</p><div className="mt-2 flex w-full flex-col gap-2"><Button onClick={onTrack}>查看我的报修</Button><Button variant="outline" onClick={onSubmitAnother}>再提交一条</Button></div></CardContent></Card></div>
}
