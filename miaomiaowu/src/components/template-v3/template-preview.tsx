import { RefreshCw, Copy } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ScrollArea } from '@/components/ui/scroll-area'

interface TemplatePreviewProps {
  content: string
  isLoading: boolean
  onRefresh: () => void
  className?: string
  title?: string
}

export function TemplatePreview({
  content,
  isLoading,
  onRefresh,
  className,
  title = '预览',
}: TemplatePreviewProps) {
  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(content)
      toast.success('已复制到剪贴板')
    } catch {
      toast.error('复制失败')
    }
  }

  return (
    <Card className={className}>
      <CardHeader className='flex flex-row items-center justify-between space-y-0 px-4 py-3'>
        <CardTitle className='text-sm font-medium'>{title}</CardTitle>
        <div className='flex items-center gap-1'>
          <Button
            variant='ghost'
            size='icon'
            className='h-8 w-8'
            onClick={handleCopy}
            disabled={!content || isLoading}
          >
            <Copy className='h-4 w-4' />
          </Button>
          <Button
            variant='ghost'
            size='icon'
            className='h-8 w-8'
            onClick={onRefresh}
            disabled={isLoading}
          >
            <RefreshCw
              className={`h-4 w-4 ${isLoading ? 'animate-spin' : ''}`}
            />
          </Button>
        </div>
      </CardHeader>
      <CardContent className='p-0'>
        <ScrollArea className='h-[calc(100vh-300px)] min-h-[300px]'>
          {isLoading ? (
            <div className='flex h-full items-center justify-center py-8'>
              <span className='text-muted-foreground'>正在生成预览...</span>
            </div>
          ) : content ? (
            <pre className='p-4 font-mono text-xs break-all whitespace-pre-wrap'>
              {content}
            </pre>
          ) : (
            <div className='flex h-full items-center justify-center py-8'>
              <span className='text-muted-foreground'>
                点击刷新按钮生成预览
              </span>
            </div>
          )}
        </ScrollArea>
      </CardContent>
    </Card>
  )
}
