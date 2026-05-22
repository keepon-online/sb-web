// Sprint 12 — 操作审计 UI
//
// Renders the operation_audit table backed by GET /api/admin/audit/operations.
// Selecting a row opens a Sheet with the full step transcript (Description +
// Metadata included as of Sprint 12's audit.go extension).
import { useMemo, useState } from 'react'
import { format } from 'date-fns'
import { AlertTriangle } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

type AuditStep = {
  id: string
  title: string
  description?: string
  kind: string
  risk: string
  target: string
  executed: boolean
  skipped_reason?: string
  error?: string
  started_at: string
  finished_at: string
  metadata?: Record<string, string>
}

type AuditRecord = {
  id: number
  plan_name: string
  dry_run: boolean
  status: 'success' | 'failed' | 'partial' | string
  started_at: string
  finished_at: string
  steps: AuditStep[]
  error?: string
  username?: string
}

type AuditListResponse = {
  items: AuditRecord[]
  count: number
}

type AcmeCertificate = {
  domain: string
  not_after: string
  days_left: number
  issuer?: string
  expired: boolean
  expiring: boolean
  parse_error?: string
}

type AcmeStatusResponse = {
  ready: boolean
  started?: string
  cache_dir: string
  email?: string
  allowed_hosts: string[]
  cached_domains?: string[]
  certificates?: AcmeCertificate[]
}

const STATUS_VARIANT: Record<
  string,
  'default' | 'destructive' | 'secondary' | 'outline'
> = {
  success: 'default',
  failed: 'destructive',
  partial: 'secondary',
}

const RISK_VARIANT: Record<
  string,
  'default' | 'destructive' | 'secondary' | 'outline'
> = {
  low: 'outline',
  medium: 'secondary',
  high: 'default',
  critical: 'destructive',
}

function formatDuration(start: string, end: string): string {
  const ms = new Date(end).getTime() - new Date(start).getTime()
  if (!Number.isFinite(ms) || ms < 0) return '-'
  if (ms < 1000) return `${ms} ms`
  return `${(ms / 1000).toFixed(2)} s`
}

function formatTimestamp(ts: string): string {
  try {
    return format(new Date(ts), 'yyyy-MM-dd HH:mm:ss')
  } catch {
    return ts
  }
}

function formatQueryError(error: unknown): string {
  if (
    typeof error === 'object' &&
    error !== null &&
    'response' in error &&
    typeof error.response === 'object' &&
    error.response !== null &&
    'data' in error.response
  ) {
    const data = error.response.data
    if (
      typeof data === 'object' &&
      data !== null &&
      'error' in data &&
      typeof data.error === 'string'
    ) {
      return data.error
    }
  }

  if (error instanceof Error && error.message) {
    return error.message
  }

  return '证书状态加载失败'
}

async function fetchAudits(params: {
  status?: string
  planName?: string
  limit?: number
}): Promise<AuditListResponse> {
  const search = new URLSearchParams()
  if (params.status) search.set('status', params.status)
  if (params.planName) search.set('plan_name', params.planName)
  if (params.limit) search.set('limit', String(params.limit))
  const qs = search.toString()
  const url = `/api/admin/audit/operations${qs ? `?${qs}` : ''}`
  const { data } = await api.get<AuditListResponse>(url)
  return data
}

async function fetchAcmeStatus(): Promise<AcmeStatusResponse> {
  const { data } = await api.get<AcmeStatusResponse>('/api/admin/acmemgr/status')
  return data
}

export function AuditPage() {
  const [statusFilter, setStatusFilter] = useState<string>('all')
  const [planFilter, setPlanFilter] = useState<string>('')
  const [selected, setSelected] = useState<AuditRecord | null>(null)

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: ['audit-operations', statusFilter, planFilter],
    queryFn: () =>
      fetchAudits({
        status: statusFilter === 'all' ? undefined : statusFilter,
        planName: planFilter || undefined,
        limit: 100,
      }),
    staleTime: 30_000,
  })

  const { data: acmeStatus, isError: isAcmeStatusError, error: acmeStatusError } =
    useQuery({
      queryKey: ['acmemgr-status'],
      queryFn: fetchAcmeStatus,
      staleTime: 30_000,
    })

  const planNames = useMemo(() => {
    if (!data?.items) return [] as string[]
    return Array.from(new Set(data.items.map((r) => r.plan_name))).sort()
  }, [data])

  const certificateWarnings = useMemo(() => {
    const certificates = acmeStatus?.certificates ?? []
    return certificates.filter(
      (cert) => cert.expired || cert.expiring || cert.parse_error
    )
  }, [acmeStatus])

  return (
    <div className='bg-background min-h-screen'>
      <main className='container mx-auto space-y-6 p-6'>
        {isAcmeStatusError && (
          <Alert variant='destructive'>
            <AlertTriangle />
            <AlertTitle>证书状态不可用</AlertTitle>
            <AlertDescription>
              {formatQueryError(acmeStatusError)}
            </AlertDescription>
          </Alert>
        )}

        {certificateWarnings.length > 0 && (
          <Alert variant='destructive'>
            <AlertTriangle />
            <AlertTitle>证书续期告警</AlertTitle>
            <AlertDescription>
              <div className='space-y-2'>
                {certificateWarnings.map((cert) => (
                  <div key={cert.domain} className='flex flex-wrap gap-x-2 gap-y-1'>
                    <span className='font-mono'>{cert.domain}</span>
                    {cert.expired && <span>已过期</span>}
                    {!cert.expired && cert.expiring && (
                      <span>将在 {cert.days_left} 天后过期</span>
                    )}
                    {cert.parse_error && <span>解析失败：{cert.parse_error}</span>}
                    {!cert.parse_error && cert.not_after && (
                      <span>到期时间：{formatTimestamp(cert.not_after)}</span>
                    )}
                  </div>
                ))}
              </div>
            </AlertDescription>
          </Alert>
        )}

        <Card>
          <CardHeader>
            <div className='flex flex-wrap items-start justify-between gap-4'>
              <div>
                <CardTitle>操作审计</CardTitle>
                <CardDescription>
                  systemops OperationPlan 执行历史 — 防火墙 / 路由 / Sing-box
                  升级 / WARP / cron / 证书等业务全覆盖
                </CardDescription>
              </div>
              <div className='flex flex-wrap items-center gap-3'>
                <Select value={statusFilter} onValueChange={setStatusFilter}>
                  <SelectTrigger className='w-[160px]'>
                    <SelectValue placeholder='状态' />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value='all'>全部状态</SelectItem>
                    <SelectItem value='success'>success</SelectItem>
                    <SelectItem value='failed'>failed</SelectItem>
                    <SelectItem value='partial'>partial</SelectItem>
                  </SelectContent>
                </Select>
                <Select
                  value={planFilter || 'all'}
                  onValueChange={(v) => setPlanFilter(v === 'all' ? '' : v)}
                >
                  <SelectTrigger className='w-[200px]'>
                    <SelectValue placeholder='Plan name' />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value='all'>全部 plan</SelectItem>
                    {planNames.map((name) => (
                      <SelectItem key={name} value={name}>
                        {name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Button variant='outline' onClick={() => refetch()}>
                  刷新
                </Button>
              </div>
            </div>
          </CardHeader>
          <CardContent>
            {isLoading ? (
              <div className='space-y-2'>
                <Skeleton className='h-10 w-full' />
                <Skeleton className='h-10 w-full' />
                <Skeleton className='h-10 w-full' />
              </div>
            ) : isError ? (
              <div className='text-destructive'>加载审计记录失败</div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className='w-[60px]'>ID</TableHead>
                    <TableHead>Plan</TableHead>
                    <TableHead className='w-[100px]'>状态</TableHead>
                    <TableHead className='w-[80px]'>Steps</TableHead>
                    <TableHead className='w-[100px]'>耗时</TableHead>
                    <TableHead className='w-[100px]'>用户</TableHead>
                    <TableHead className='w-[160px]'>开始时间</TableHead>
                    <TableHead className='w-[80px]'>操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {data?.items?.length ? (
                    data.items.map((rec) => (
                      <TableRow key={rec.id}>
                        <TableCell>{rec.id}</TableCell>
                        <TableCell>
                          <span className='font-mono text-sm'>
                            {rec.plan_name}
                          </span>
                          {rec.dry_run && (
                            <Badge variant='outline' className='ml-2'>
                              dry-run
                            </Badge>
                          )}
                        </TableCell>
                        <TableCell>
                          <Badge
                            variant={STATUS_VARIANT[rec.status] ?? 'outline'}
                          >
                            {rec.status}
                          </Badge>
                        </TableCell>
                        <TableCell>{rec.steps?.length ?? 0}</TableCell>
                        <TableCell>
                          {formatDuration(rec.started_at, rec.finished_at)}
                        </TableCell>
                        <TableCell className='text-muted-foreground text-sm'>
                          {rec.username || '-'}
                        </TableCell>
                        <TableCell className='font-mono text-sm'>
                          {formatTimestamp(rec.started_at)}
                        </TableCell>
                        <TableCell>
                          <Button
                            variant='ghost'
                            size='sm'
                            onClick={() => setSelected(rec)}
                          >
                            查看
                          </Button>
                        </TableCell>
                      </TableRow>
                    ))
                  ) : (
                    <TableRow>
                      <TableCell
                        colSpan={8}
                        className='text-muted-foreground py-8 text-center'
                      >
                        暂无审计记录
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>
      </main>

      <Sheet
        open={!!selected}
        onOpenChange={(open) => !open && setSelected(null)}
      >
        <SheetContent className='flex w-full flex-col overflow-hidden sm:max-w-2xl'>
          <SheetHeader>
            <SheetTitle>
              {selected ? `审计 #${selected.id} — ${selected.plan_name}` : ''}
            </SheetTitle>
            <SheetDescription>
              {selected && (
                <span className='flex flex-wrap items-center gap-2'>
                  <Badge variant={STATUS_VARIANT[selected.status] ?? 'outline'}>
                    {selected.status}
                  </Badge>
                  {selected.dry_run && <Badge variant='outline'>dry-run</Badge>}
                  <span className='text-xs'>
                    {formatTimestamp(selected.started_at)} →{' '}
                    {formatTimestamp(selected.finished_at)} (
                    {formatDuration(selected.started_at, selected.finished_at)})
                  </span>
                  {selected.username && (
                    <span className='text-xs'>by {selected.username}</span>
                  )}
                </span>
              )}
            </SheetDescription>
          </SheetHeader>
          <ScrollArea className='mt-4 flex-1 pr-4'>
            <div className='space-y-3'>
              {selected?.error && (
                <Card>
                  <CardHeader>
                    <CardTitle className='text-destructive text-sm'>
                      Plan 错误
                    </CardTitle>
                  </CardHeader>
                  <CardContent>
                    <pre className='font-mono text-xs whitespace-pre-wrap'>
                      {selected.error}
                    </pre>
                  </CardContent>
                </Card>
              )}
              {selected?.steps?.map((step, idx) => (
                <Card key={`${step.id}-${idx}`}>
                  <CardHeader className='pb-2'>
                    <div className='flex items-center justify-between gap-2'>
                      <CardTitle className='font-mono text-base'>
                        {idx + 1}. {step.id}
                      </CardTitle>
                      <div className='flex items-center gap-2'>
                        <Badge variant={RISK_VARIANT[step.risk] ?? 'outline'}>
                          {step.risk}
                        </Badge>
                        <Badge variant='outline'>{step.kind}</Badge>
                        {step.executed && !step.error && (
                          <Badge>执行成功</Badge>
                        )}
                        {!step.executed && step.skipped_reason && (
                          <Badge variant='secondary'>
                            跳过: {step.skipped_reason}
                          </Badge>
                        )}
                        {step.error && (
                          <Badge variant='destructive'>失败</Badge>
                        )}
                      </div>
                    </div>
                    <CardDescription className='text-xs'>
                      {step.title}
                    </CardDescription>
                  </CardHeader>
                  <CardContent className='space-y-2'>
                    <div className='text-xs'>
                      <span className='text-muted-foreground'>Target: </span>
                      <span className='font-mono'>{step.target}</span>
                    </div>
                    {step.description && (
                      <div className='text-xs'>
                        <span className='text-muted-foreground'>Command: </span>
                        <span className='font-mono break-all'>
                          {step.description}
                        </span>
                      </div>
                    )}
                    {step.metadata && Object.keys(step.metadata).length > 0 && (
                      <div className='text-xs'>
                        <div className='text-muted-foreground mb-1'>
                          Metadata
                        </div>
                        <div className='bg-muted/40 space-y-1 rounded p-2'>
                          {Object.entries(step.metadata).map(([k, v]) => (
                            <div
                              key={k}
                              className='grid grid-cols-[120px_1fr] gap-2'
                            >
                              <span className='text-muted-foreground font-mono'>
                                {k}
                              </span>
                              <span className='font-mono break-all'>{v}</span>
                            </div>
                          ))}
                        </div>
                      </div>
                    )}
                    {step.error && (
                      <>
                        <Separator />
                        <div className='text-xs'>
                          <div className='text-destructive mb-1'>Error</div>
                          <pre className='font-mono text-xs whitespace-pre-wrap'>
                            {step.error}
                          </pre>
                        </div>
                      </>
                    )}
                    <div className='text-muted-foreground text-xs'>
                      {formatTimestamp(step.started_at)} (
                      {formatDuration(step.started_at, step.finished_at)})
                    </div>
                  </CardContent>
                </Card>
              ))}
            </div>
          </ScrollArea>
        </SheetContent>
      </Sheet>
    </div>
  )
}
