import { useState, useRef, useCallback, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  RefreshCw,
  Download,
  CheckCircle,
  AlertTriangle,
  ExternalLink,
  Circle,
} from 'lucide-react'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'
import { api } from '@/lib/api'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Progress } from '@/components/ui/progress'

// 轻量 markdown 渲染：处理 GitHub release notes 常见格式
function processInline(text: string): ReactNode[] {
  const parts: ReactNode[] = []
  const pattern =
    /(\*\*(.+?)\*\*)|(\[([^\]]+)\]\(([^)]+)\))|(`([^`]+)`)|(https?:\/\/[^\s)<]+)/g
  let lastIndex = 0
  let match: RegExpExecArray | null
  let key = 0

  while ((match = pattern.exec(text)) !== null) {
    if (match.index > lastIndex) parts.push(text.slice(lastIndex, match.index))
    if (match[2]) {
      parts.push(<strong key={key++}>{match[2]}</strong>)
    } else if (match[4] && match[5]) {
      parts.push(
        <a
          key={key++}
          href={match[5]}
          target='_blank'
          rel='noopener noreferrer'
          className='text-primary hover:underline'
        >
          {match[4]}
        </a>
      )
    } else if (match[7]) {
      parts.push(
        <code key={key++} className='bg-muted rounded px-1 text-xs'>
          {match[7]}
        </code>
      )
    } else if (match[8]) {
      parts.push(
        <a
          key={key++}
          href={match[8]}
          target='_blank'
          rel='noopener noreferrer'
          className='text-primary break-all hover:underline'
        >
          {match[8]}
        </a>
      )
    }
    lastIndex = match.index + match[0].length
  }
  if (lastIndex < text.length) parts.push(text.slice(lastIndex))
  return parts
}

function ReleaseNotes({ text }: { text: string }) {
  return (
    <div className='text-muted-foreground space-y-1.5 text-sm'>
      {text.split('\n').map((line, i) => {
        if (/^#{1,3} /.test(line)) {
          return (
            <h4
              key={i}
              className='text-foreground mt-2 font-semibold first:mt-0'
            >
              {processInline(line.replace(/^#{1,3} /, ''))}
            </h4>
          )
        }
        if (/^[*\-] /.test(line)) {
          return (
            <div key={i} className='ml-1 flex gap-1.5'>
              <span className='shrink-0'>•</span>
              <span className='min-w-0'>
                {processInline(line.replace(/^[*\-] /, ''))}
              </span>
            </div>
          )
        }
        if (!line.trim()) return <div key={i} className='h-1' />
        return <p key={i}>{processInline(line)}</p>
      })}
    </div>
  )
}

interface UpdateDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

interface UpdateInfo {
  current_version: string
  latest_version: string
  has_update: boolean
  release_url: string
  download_url: string
  release_notes: string
}

interface UpdateProgress {
  step:
    | 'checking'
    | 'downloading'
    | 'backing_up'
    | 'replacing'
    | 'restarting'
    | 'done'
    | 'error'
  progress: number
  message: string
}

const STEPS = [
  { key: 'checking', label: '检查版本' },
  { key: 'downloading', label: '下载更新' },
  { key: 'backing_up', label: '备份当前版本' },
  { key: 'replacing', label: '替换文件' },
  { key: 'restarting', label: '重启服务' },
] as const

export function UpdateDialog({ open, onOpenChange }: UpdateDialogProps) {
  const [isUpdating, setIsUpdating] = useState(false)
  const [updateProgress, setUpdateProgress] = useState<UpdateProgress | null>(
    null
  )
  const updateCompleteRef = useRef(false)
  const { auth } = useAuthStore()

  // Check for updates
  const {
    data: updateInfo,
    isLoading,
    refetch,
    isRefetching,
  } = useQuery({
    queryKey: ['update-check'],
    queryFn: async () => {
      const response = await api.get('/api/admin/update/check')
      return response.data as UpdateInfo
    },
    enabled: open,
    staleTime: 0,
    retry: 1,
  })

  // Start update with SSE using fetch (more reliable than EventSource for auth)
  const startUpdate = useCallback(async () => {
    setIsUpdating(true)
    setUpdateProgress(null)
    updateCompleteRef.current = false

    try {
      const response = await fetch('/api/admin/update/apply-sse', {
        method: 'GET',
        headers: {
          'MM-Authorization': auth.accessToken || '',
        },
      })

      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`)
      }

      const reader = response.body?.getReader()
      if (!reader) {
        throw new Error('无法读取响应流')
      }

      const decoder = new TextDecoder()
      let buffer = ''

      while (true) {
        const { done, value } = await reader.read()
        if (done) break

        buffer += decoder.decode(value, { stream: true })
        const lines = buffer.split('\n')
        buffer = lines.pop() || ''

        for (const line of lines) {
          if (line.startsWith('data: ')) {
            try {
              const progress = JSON.parse(line.slice(6)) as UpdateProgress
              setUpdateProgress(progress)

              if (progress.step === 'done') {
                updateCompleteRef.current = true
                toast.success('更新成功，页面将在 3 秒后刷新')
                setTimeout(() => {
                  window.location.reload()
                }, 3000)
                return
              } else if (progress.step === 'error') {
                updateCompleteRef.current = true
                setIsUpdating(false)
                toast.error(progress.message)
                return
              }
            } catch {
              // Ignore parse errors
            }
          }
        }
      }

      // Stream ended without done/error
      if (!updateCompleteRef.current) {
        setIsUpdating(false)
        toast.error('连接意外关闭')
      }
    } catch (error) {
      if (!updateCompleteRef.current) {
        setIsUpdating(false)
        toast.error(
          `更新失败: ${error instanceof Error ? error.message : '未知错误'}`
        )
      }
    }
  }, [auth.accessToken])

  // Cleanup on close
  const handleOpenChange = (newOpen: boolean) => {
    onOpenChange(newOpen)
  }

  const isCheckingOrRefetching = isLoading || isRefetching

  // Get current step index for UI
  const currentStepIndex = STEPS.findIndex(
    (s) => s.key === updateProgress?.step
  )

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className='overflow-hidden sm:max-w-md'>
        <DialogHeader>
          <DialogTitle className='flex items-center gap-2'>
            <RefreshCw className='size-5' /> 检查更新
          </DialogTitle>
          <DialogDescription>检查是否有新版本可用</DialogDescription>
        </DialogHeader>

        <div className='space-y-4'>
          {isCheckingOrRefetching ? (
            <div className='py-8 text-center'>
              <RefreshCw className='text-primary mx-auto mb-3 size-8 animate-spin' />
              <p className='text-muted-foreground text-sm'>正在检查更新...</p>
            </div>
          ) : updateInfo?.has_update ? (
            <div className='space-y-4'>
              <div className='flex items-center gap-2 text-amber-500'>
                <AlertTriangle className='size-5' />
                <span className='font-medium'>发现新版本！</span>
              </div>

              <div className='bg-muted/50 space-y-2 rounded-lg p-3'>
                <div className='flex justify-between text-sm'>
                  <span className='text-muted-foreground'>当前版本</span>
                  <span className='font-mono'>
                    v{updateInfo.current_version}
                  </span>
                </div>
                <div className='flex justify-between text-sm'>
                  <span className='text-muted-foreground'>最新版本</span>
                  <span className='font-mono text-green-600'>
                    v{updateInfo.latest_version}
                  </span>
                </div>
              </div>

              {updateInfo.release_notes && !isUpdating && (
                <div className='space-y-2 overflow-hidden'>
                  <p className='text-sm font-medium'>更新内容：</p>
                  <div className='bg-muted/30 max-h-48 overflow-x-hidden overflow-y-auto rounded-lg p-3'>
                    <ReleaseNotes text={updateInfo.release_notes} />
                  </div>
                </div>
              )}

              {/* Progress UI */}
              {isUpdating && (
                <div className='space-y-4'>
                  <div className='space-y-2'>
                    {STEPS.map((step, index) => {
                      const isCompleted = index < currentStepIndex
                      const isCurrent = step.key === updateProgress?.step
                      const isPending =
                        index > currentStepIndex || currentStepIndex === -1

                      return (
                        <div key={step.key} className='flex items-center gap-3'>
                          {isCompleted ? (
                            <CheckCircle className='size-5 shrink-0 text-green-500' />
                          ) : isCurrent ? (
                            <RefreshCw className='text-primary size-5 shrink-0 animate-spin' />
                          ) : (
                            <Circle className='text-muted-foreground size-5 shrink-0' />
                          )}
                          <span
                            className={
                              isCurrent
                                ? 'font-medium'
                                : isPending
                                  ? 'text-muted-foreground'
                                  : ''
                            }
                          >
                            {step.label}
                          </span>
                          {isCurrent &&
                            step.key === 'downloading' &&
                            updateProgress && (
                              <span className='ml-auto font-mono text-sm'>
                                {updateProgress.progress}%
                              </span>
                            )}
                        </div>
                      )
                    })}
                  </div>

                  {/* Progress bar for downloading */}
                  {updateProgress?.step === 'downloading' && (
                    <Progress value={updateProgress.progress} className='h-2' />
                  )}

                  <div className='rounded-lg bg-blue-50 p-3 dark:bg-blue-950/30'>
                    <p className='text-sm text-blue-600 dark:text-blue-400'>
                      {updateProgress?.message || '正在准备更新...'}
                    </p>
                  </div>
                </div>
              )}

              {!isUpdating && (
                <div className='flex flex-col gap-2'>
                  <Button
                    onClick={startUpdate}
                    disabled={isUpdating || !updateInfo.download_url}
                    className='w-full'
                  >
                    <Download className='mr-2 size-4' />
                    立即更新
                  </Button>

                  {!updateInfo.download_url && (
                    <p className='text-destructive text-center text-xs'>
                      未找到适合当前系统的下载文件
                    </p>
                  )}

                  {updateInfo.release_url && (
                    <Button
                      variant='outline'
                      className='w-full'
                      onClick={() =>
                        window.open(updateInfo.release_url, '_blank')
                      }
                    >
                      <ExternalLink className='mr-2 size-4' />
                      查看 GitHub Release
                    </Button>
                  )}
                </div>
              )}
            </div>
          ) : (
            <div className='py-8 text-center'>
              <CheckCircle className='mx-auto mb-3 size-12 text-green-500' />
              <p className='text-lg font-medium'>已是最新版本</p>
              <p className='text-muted-foreground mt-1 text-sm'>
                当前版本：v{updateInfo?.current_version}
              </p>
            </div>
          )}

          <Button
            variant='outline'
            onClick={() => refetch()}
            disabled={isCheckingOrRefetching || isUpdating}
            className='w-full'
          >
            <RefreshCw
              className={`mr-2 size-4 ${isCheckingOrRefetching ? 'animate-spin' : ''}`}
            />
            重新检查
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
