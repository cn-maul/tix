import { Badge } from '@/components/ui/badge'

// 工单状态徽标：列表与详情页共用同一实现，避免样式重复定义。
export default function StatusBadge({ status }: { status: 0 | 1 }) {
  return (
    <Badge
      variant="secondary"
      className={
        status === 0
          ? 'bg-amber-100 text-amber-700 dark:bg-amber-500/15 dark:text-amber-400'
          : 'bg-emerald-100 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-400'
      }
    >
      {status === 0 ? '待处理' : '已处理'}
    </Badge>
  )
}
