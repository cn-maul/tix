import { Component, type ReactNode } from 'react'
import { AlertTriangle } from 'lucide-react'
import { Button } from '@/components/ui/button'

interface Props {
  children: ReactNode
}

interface State {
  hasError: boolean
  error: Error | null
}

export class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props)
    this.state = { hasError: false, error: null }
  }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error }
  }

  componentDidCatch(error: Error, errorInfo: React.ErrorInfo) {
    console.error('ErrorBoundary caught:', error, errorInfo)
  }

  render() {
    if (this.state.hasError) {
      return (
        <div className="flex min-h-screen flex-col items-center justify-center p-8 text-center">
          <AlertTriangle className="mb-4 size-12 text-destructive" />
          <h1 className="mb-2 text-xl font-semibold">页面出错了</h1>
          <p className="mb-4 text-muted-foreground">
            {this.state.error?.message || '发生了未知错误'}
          </p>
          <Button onClick={() => window.location.reload()}>刷新页面</Button>
        </div>
      )
    }
    return this.props.children
  }
}
