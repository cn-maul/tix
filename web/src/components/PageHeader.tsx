import type { ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

export default function PageHeader({
  title,
  description,
  action,
  onBack,
  className,
}: {
  title: ReactNode
  description?: ReactNode
  action?: ReactNode
  onBack?: () => void
  className?: string
}) {
  return (
    <div className={cn('flex flex-wrap items-start justify-between gap-3', className)}>
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-3">
          {onBack && (
            <Button variant="ghost" size="sm" onClick={onBack}>
              返回
            </Button>
          )}
          <h1 className="text-xl font-semibold">{title}</h1>
        </div>
        {description && <p className="mt-1 text-sm text-muted-foreground">{description}</p>}
      </div>
      {action && <div className="flex flex-wrap items-center gap-2">{action}</div>}
    </div>
  )
}
