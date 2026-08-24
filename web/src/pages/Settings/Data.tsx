import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'

// 数据与备份：CSV 导出入口已迁移至工单列表页（导出跟随当前筛选条件），
// 本页仅保留存储与备份说明，避免同类功能重复入口。
export default function Data() {
  return (
    <div className="space-y-4">
      <h1 className="text-xl font-semibold">数据与备份</h1>
      <p className="text-sm text-muted-foreground">
        数据存储在 SQLite 文件{' '}
        <code className="rounded bg-muted px-1.5 py-0.5 font-mono">tix.db</code>{' '}
        中（WAL 模式下还有 <code className="rounded bg-muted px-1.5 py-0.5 font-mono">-wal</code> /{' '}
        <code className="rounded bg-muted px-1.5 py-0.5 font-mono">-shm</code> 伴生文件），建议先停机再整目录备份。
      </p>

      <Card>
        <CardHeader>
          <CardTitle>备份建议</CardTitle>
          <CardDescription>两种常用方式</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3 text-sm text-muted-foreground">
          <p>
            <span className="font-medium text-foreground">停机复制：</span>
            停止 tix 进程后复制 <code className="rounded bg-muted px-1.5 py-0.5 font-mono">tix.db</code>（及伴生文件）到安全位置。
          </p>
          <p>
            <span className="font-medium text-foreground">在线快照：</span>
            不停机执行{' '}
            <code className="rounded bg-muted px-1.5 py-0.5 font-mono">sqlite3 tix.db "VACUUM INTO '/backup/tix-snapshot.db'"</code>
            ，得到一致性强、体积更小的单文件快照。
          </p>
          <hr />
          <p>
            工单清单的 CSV 导出已移至「工单列表」页：在列表中按状态 / 分类 / 时间范围 / 负责人筛选后，
            点击工具栏的「导出 CSV」即可按当前条件导出。
          </p>
        </CardContent>
      </Card>
    </div>
  )
}
