// 页面级加载态统一组件：替代各页手写 spinner（尺寸/样式保持一致）。
// 表格类数据仍使用 DataTable 内置的骨架屏。
export default function PageSpinner({ label = '加载中…' }: { label?: string }) {
  return (
    <div className="flex h-64 flex-col items-center justify-center gap-2" role="status" aria-label={label}>
      <div className="size-8 animate-spin rounded-full border-2 border-muted border-t-primary" />
      <span className="text-sm text-muted-foreground">{label}</span>
    </div>
  )
}
