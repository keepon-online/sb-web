// @ts-nocheck
import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute, redirect } from '@tanstack/react-router'
import { load as parseYAML } from 'js-yaml'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'
import { api } from '@/lib/api'
import { handleServerError } from '@/lib/handle-server-error'
import { profileQueryFn } from '@/lib/profile'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'
import { Textarea } from '@/components/ui/textarea'

// @ts-ignore - retained simple route definition
export const Route = createFileRoute('/rules')({
  beforeLoad: () => {
    const token = useAuthStore.getState().auth.accessToken
    if (!token) {
      throw redirect({ to: '/' })
    }
  },
  validateSearch: (search: Record<string, unknown>) => {
    return {
      file: (search.file as string) || undefined,
    }
  },
  component: RulesPage,
})

function RulesPage() {
  const { auth } = useAuthStore()
  const queryClient = useQueryClient()
  const search = Route.useSearch()
  const [selectedFile, setSelectedFile] = useState<string | null>(
    search.file || null
  )
  const [editorValue, setEditorValue] = useState('')
  const [isDirty, setIsDirty] = useState(false)
  const [validationError, setValidationError] = useState<string | null>(null)

  const {
    data: profile,
    isLoading: profileLoading,
    isError: profileError,
  } = useQuery({
    queryKey: ['profile'],
    queryFn: profileQueryFn,
    enabled: Boolean(auth.accessToken),
    staleTime: 5 * 60 * 1000,
  })

  const isAdmin = Boolean(profile?.is_admin)

  const dateFormatter = useMemo(
    () =>
      new Intl.DateTimeFormat('zh-CN', {
        dateStyle: 'medium',
        timeStyle: 'short',
        hour12: false,
      }),
    []
  )

  const listQuery = useQuery({
    queryKey: ['rule-files'],
    queryFn: async () => {
      const response = await api.get('/api/admin/rules')
      return response.data as {
        files: Array<{
          name: string
          size: number
          mod_time: number
          latest_version: number
        }>
      }
    },
    enabled: Boolean(auth.accessToken && isAdmin),
    staleTime: 60 * 1000,
  })

  useEffect(() => {
    if (!isAdmin) return
    if (selectedFile) return
    if (search.file) return // Don't auto-select if URL has a file parameter
    const first = listQuery.data?.files?.[0]?.name
    if (first) {
      setSelectedFile(first)
    }
  }, [isAdmin, listQuery.data, selectedFile, search.file])

  const detailQuery = useQuery({
    queryKey: ['rule-file', selectedFile],
    queryFn: async () => {
      if (!selectedFile) return null
      const response = await api.get(
        `/api/admin/rules/${encodeURIComponent(selectedFile)}`
      )
      return response.data as {
        name: string
        content: string
        latest_version: number
      }
    },
    enabled: Boolean(selectedFile && auth.accessToken && isAdmin),
    refetchOnWindowFocus: false,
  })

  useEffect(() => {
    if (!detailQuery.data) return
    setEditorValue(detailQuery.data.content ?? '')
    setIsDirty(false)
    setValidationError(null)
  }, [detailQuery.data])

  const historyQuery = useQuery({
    queryKey: ['rule-history', selectedFile],
    queryFn: async () => {
      if (!selectedFile) return { history: [] as Array<any> }
      const response = await api.get(
        `/api/admin/rules/${encodeURIComponent(selectedFile)}/history`
      )
      return response.data as {
        history: Array<{
          version: number
          content: string
          created_by: string
          created_at: string
        }>
      }
    },
    enabled: Boolean(selectedFile && auth.accessToken && isAdmin),
  })

  const saveMutation = useMutation({
    mutationFn: async (payload: { file: string; content: string }) => {
      const response = await api.put(
        `/api/admin/rules/${encodeURIComponent(payload.file)}`,
        {
          content: payload.content,
        }
      )
      return response.data as { version: number }
    },
    onSuccess: (_, variables) => {
      toast.success('规则已保存')
      setIsDirty(false)
      setValidationError(null)
      queryClient.invalidateQueries({ queryKey: ['rule-files'] })
      queryClient.invalidateQueries({ queryKey: ['rule-file', variables.file] })
      queryClient.invalidateQueries({
        queryKey: ['rule-history', variables.file],
      })
    },
    onError: (error) => {
      handleServerError(error)
    },
  })

  const isLoadingContent = detailQuery.isLoading || detailQuery.isFetching
  const files = listQuery.data?.files ?? []
  const historyList = historyQuery.data?.history ?? []

  const subscriptionLabelMap = useMemo(
    () =>
      ({
        'subscribe.yaml': ['Clash Mobile'],
        'subscribe-openclash-redirhost.yaml': ['OpenClash-RedirHost'],
        'subscribe-openclash-fakeip.yaml': ['OpenClash-Fakeip'],
      }) as Record<string, string[]>,
    []
  )

  useEffect(() => {
    if (!selectedFile) {
      setValidationError(null)
      return
    }
    if (isLoadingContent) {
      return
    }

    const timer = setTimeout(() => {
      const trimmed = editorValue.trim()
      if (!trimmed) {
        setValidationError('内容不能为空')
        return
      }

      try {
        parseYAML(editorValue)
        setValidationError(null)
      } catch (error) {
        const message = error instanceof Error ? error.message : 'YAML 解析失败'
        setValidationError(message)
      }
    }, 300)

    return () => {
      clearTimeout(timer)
    }
  }, [editorValue, selectedFile, isLoadingContent])

  const handleSelectFile = (name: string) => {
    if (name === selectedFile) return
    setSelectedFile(name)
    setEditorValue('')
    setIsDirty(false)
    setValidationError(null)
  }

  const handleSave = () => {
    if (!selectedFile) return
    try {
      parseYAML(editorValue || '')
      setValidationError(null)
    } catch (error) {
      const message = error instanceof Error ? error.message : 'YAML 解析失败'
      setValidationError(message)
      toast.error('保存失败，YAML 格式错误')
      return
    }

    saveMutation.mutate({ file: selectedFile, content: editorValue })
  }

  const handleReset = () => {
    if (!detailQuery.data) return
    setEditorValue(detailQuery.data.content ?? '')
    setIsDirty(false)
    setValidationError(null)
  }

  if (profileLoading) {
    return (
      <div className='bg-background min-h-svh'>
        <main className='mx-auto w-full max-w-6xl px-4 py-8 sm:px-6'>
          <Skeleton className='h-48 w-full' />
        </main>
      </div>
    )
  }

  if (!isAdmin || profileError) {
    return (
      <div className='bg-background min-h-svh'>
        <main className='mx-auto flex w-full max-w-3xl flex-col items-center justify-center gap-4 px-4 py-20 text-center sm:px-6'>
          <Card className='w-full border-dashed shadow-none'>
            <CardHeader>
              <CardTitle>权限不足</CardTitle>
              <CardDescription>
                只有管理员可以访问规则配置页面。
              </CardDescription>
            </CardHeader>
          </Card>
        </main>
      </div>
    )
  }

  return (
    <div className='bg-background min-h-svh'>
      <main className='mx-auto w-full max-w-6xl px-4 py-8 sm:px-6'>
        <section className='space-y-4'>
          <h1 className='text-3xl font-semibold tracking-tight'>规则配置</h1>
          <p className='text-muted-foreground'>
            查看、编辑并保存订阅规则，支持版本历史留存。
          </p>
        </section>

        <section className='mt-8 grid gap-6 lg:grid-cols-[320px_1fr]'>
          <Card>
            <CardHeader>
              <CardTitle className='text-base'>规则文件</CardTitle>
              <CardDescription>选择需要编辑的 YAML 文件</CardDescription>
            </CardHeader>
            <CardContent>
              {listQuery.isLoading ? (
                <div className='space-y-3'>
                  {Array.from({ length: 3 }).map((_, idx) => (
                    <Skeleton key={idx} className='h-10 w-full rounded-md' />
                  ))}
                </div>
              ) : files.length === 0 ? (
                <p className='text-muted-foreground text-sm'>
                  未找到任何 YAML 文件。
                </p>
              ) : (
                <div className='space-y-2'>
                  {files.map((file) => {
                    const labels = subscriptionLabelMap[file.name]
                    const displayName = labels?.length
                      ? labels.join(' / ')
                      : file.name
                    return (
                      <Button
                        key={file.name}
                        variant={
                          file.name === selectedFile ? 'secondary' : 'ghost'
                        }
                        className={cn(
                          'w-full justify-between text-left font-normal'
                        )}
                        onClick={() => handleSelectFile(file.name)}
                        title={file.name}
                      >
                        <span>{displayName}</span>
                        {file.latest_version > 0 ? (
                          <Badge variant='outline'>
                            v{file.latest_version}
                          </Badge>
                        ) : null}
                      </Button>
                    )
                  })}
                </div>
              )}
            </CardContent>
          </Card>

          <div className='space-y-6'>
            <Card>
              <CardHeader className='space-y-2'>
                <CardTitle className='flex items-center justify-between'>
                  <span>{selectedFile ?? '未选择文件'}</span>
                  <span className='flex items-center gap-2'>
                    {detailQuery.data?.latest_version ? (
                      <Badge variant='secondary'>
                        最新版本 v{detailQuery.data.latest_version}
                      </Badge>
                    ) : null}
                  </span>
                </CardTitle>
                <CardDescription>
                  编辑内容时会自动校验 YAML 格式
                </CardDescription>
              </CardHeader>
              <CardContent className='space-y-4'>
                <div className='flex flex-wrap items-center gap-3'>
                  <Button
                    size='sm'
                    onClick={handleSave}
                    disabled={
                      !selectedFile ||
                      !isDirty ||
                      saveMutation.isPending ||
                      isLoadingContent
                    }
                  >
                    {saveMutation.isPending ? '保存中...' : '保存修改'}
                  </Button>
                  <Button
                    size='sm'
                    variant='outline'
                    disabled={
                      !isDirty || isLoadingContent || saveMutation.isPending
                    }
                    onClick={handleReset}
                  >
                    还原修改
                  </Button>
                  <span className='text-muted-foreground text-xs'>
                    保存后会生成新的历史版本
                  </span>
                </div>
                {validationError ? (
                  <div className='border-destructive/60 bg-destructive/10 text-destructive rounded-md border p-3 text-sm'>
                    {validationError}
                  </div>
                ) : null}

                <div className='bg-muted/20 rounded-lg border'>
                  {isLoadingContent ? (
                    <div className='space-y-3 p-4'>
                      <Skeleton className='h-4 w-3/4' />
                      <Skeleton className='h-4 w-full' />
                      <Skeleton className='h-4 w-5/6' />
                      <Skeleton className='h-4 w-4/6' />
                    </div>
                  ) : (
                    <Textarea
                      value={editorValue}
                      onChange={(event) => {
                        const nextValue = event.target.value
                        setEditorValue(nextValue)
                        setIsDirty(
                          nextValue !== (detailQuery.data?.content ?? '')
                        )
                        if (validationError) {
                          setValidationError(null)
                        }
                      }}
                      className='min-h-[420px] font-mono text-sm'
                      disabled={!selectedFile || saveMutation.isPending}
                      spellCheck={false}
                    />
                  )}
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className='text-base'>历史版本</CardTitle>
                <CardDescription>最近保存的版本会在此展示</CardDescription>
              </CardHeader>
              <CardContent>
                {historyQuery.isLoading ? (
                  <div className='space-y-3'>
                    {Array.from({ length: 3 }).map((_, index) => (
                      <Skeleton
                        key={index}
                        className='h-12 w-full rounded-md'
                      />
                    ))}
                  </div>
                ) : historyList.length === 0 ? (
                  <p className='text-muted-foreground text-sm'>
                    暂无历史记录，保存后会自动生成版本。
                  </p>
                ) : (
                  <ScrollArea className='h-64 pr-3'>
                    <div className='space-y-4'>
                      {historyList.map((item) => (
                        <div
                          key={item.version}
                          className='space-y-2 rounded-md border p-3'
                        >
                          <div className='flex items-center justify-between text-sm font-medium'>
                            <span>版本 v{item.version}</span>
                            <Badge variant='outline'>{item.created_by}</Badge>
                          </div>
                          <div className='text-muted-foreground text-xs'>
                            {item.created_at
                              ? dateFormatter.format(new Date(item.created_at))
                              : '时间未知'}
                          </div>
                          <Separator className='my-2' />
                          <pre className='bg-muted/40 max-h-32 overflow-auto rounded p-2 text-xs break-words whitespace-pre-wrap'>
                            {item.content}
                          </pre>
                        </div>
                      ))}
                    </div>
                  </ScrollArea>
                )}
              </CardContent>
            </Card>
          </div>
        </section>
      </main>
    </div>
  )
}
