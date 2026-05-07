import { createFileRoute } from '@tanstack/react-router'
import {
  CheckCircle2,
  Copy,
  FileJson,
  KeyRound,
  Link2,
  RefreshCw,
  Save,
  Server,
  Shield,
  Trash2,
} from 'lucide-react'
import { useEffect, useMemo, useState, type ReactNode } from 'react'
import axios, { AxiosError } from 'axios'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

interface Config {
  id: number
  name: string
  protocol: string
  port: number
  enabled: boolean
  created_at: string
}

interface PortStatus {
  used_ports: number[]
  available_count: number
  range: {
    min: number
    max: number
  }
}

interface GenerateResponse {
  success: boolean
  message: string
  config?: Record<string, unknown>
  link?: string
  links?: Record<string, string>
  port?: number
}

interface GeneratedResult {
  protocol: string
  name: string
  port: number
  config: Record<string, unknown>
  links: Record<string, string>
}

type GenerateMode = 'server' | 'client'

interface ServerForm {
  externalHost: string
  hostname: string
  uuid: string
  password: string
  realitySNI: string
  realityPrivateKey: string
  realityPublicKey: string
  realityShortID: string
  websocketPath: string
  certificatePath: string
  privateKeyPath: string
  vlessRealityPort: number
  vmessWebSocketPort: number
  hysteria2Port: number
  tuicPort: number
  anytlsPort: number
}

const defaultServerForm: ServerForm = {
  externalHost: '',
  hostname: '',
  uuid: '',
  password: '',
  realitySNI: 'apple.com',
  realityPrivateKey: '',
  realityPublicKey: '',
  realityShortID: '',
  websocketPath: '/vmessws',
  certificatePath: '/etc/s-box/cert.pem',
  privateKeyPath: '/etc/s-box/private.key',
  vlessRealityPort: 10000,
  vmessWebSocketPort: 10001,
  hysteria2Port: 10002,
  tuicPort: 10003,
  anytlsPort: 10004,
}

const serverPortFields = [
  ['vlessRealityPort', 'VLESS Reality'],
  ['vmessWebSocketPort', 'VMess WS'],
  ['hysteria2Port', 'Hysteria2'],
  ['tuicPort', 'TUIC'],
  ['anytlsPort', 'AnyTLS'],
] as const

const singleProtocols = [
  ['vless', 'VLESS Reality'],
  ['vmess', 'VMess WebSocket'],
  ['hysteria2', 'Hysteria2'],
  ['tuic', 'TUIC'],
  ['anytls', 'AnyTLS'],
] as const

export const Route = createFileRoute('/singbox/config')({
  component: SingboxConfigPage,
})

function randomHex(bytes: number) {
  const values = new Uint8Array(bytes)
  crypto.getRandomValues(values)
  return Array.from(values, (value) => value.toString(16).padStart(2, '0')).join('')
}

function randomToken(bytes = 18) {
  return btoa(randomHex(bytes)).replace(/[=+/]/g, '').slice(0, 24)
}

function getErrorMessage(err: unknown, fallback: string) {
  if (err instanceof AxiosError) {
    const data = err.response?.data as { error?: string; message?: string } | undefined
    return data?.error || data?.message || fallback
  }
  if (err instanceof Error) return err.message
  return fallback
}

function normalizeLinks(response: GenerateResponse) {
  if (response.links && Object.keys(response.links).length > 0) return response.links
  if (response.link) return { link: response.link }
  return {}
}

function SingboxConfigPage() {
  const [configs, setConfigs] = useState<Config[]>([])
  const [portStatus, setPortStatus] = useState<PortStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [generating, setGenerating] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [mode, setMode] = useState<GenerateMode>('server')
  const [generatedResult, setGeneratedResult] = useState<GeneratedResult | null>(null)

  const [serverForm, setServerForm] = useState<ServerForm>({
    ...defaultServerForm,
    uuid: crypto.randomUUID(),
    password: randomToken(),
    realityShortID: randomHex(4),
  })

  const [selectedProtocol, setSelectedProtocol] = useState('vless')
  const [serverAddress, setServerAddress] = useState('')
  const [serverPort, setServerPort] = useState(443)
  const [domain, setDomain] = useState('')
  const [autoPort, setAutoPort] = useState(true)

  const generatedJSON = useMemo(() => {
    if (!generatedResult?.config) return ''
    return JSON.stringify(generatedResult.config, null, 2)
  }, [generatedResult])

  useEffect(() => {
    loadConfigs()
    loadPortStatus()
  }, [])

  const loadConfigs = async () => {
    try {
      const response = await axios.get('/api/admin/singbox/config/list')
      setConfigs(response.data.configs || [])
    } catch (err) {
      console.error('加载配置失败:', err)
    } finally {
      setLoading(false)
    }
  }

  const loadPortStatus = async () => {
    try {
      const response = await axios.get('/api/admin/singbox/port/status')
      setPortStatus(response.data)
    } catch (err) {
      console.error('加载端口状态失败:', err)
    }
  }

  const updateServerForm = <K extends keyof ServerForm>(
    key: K,
    value: ServerForm[K],
  ) => {
    setServerForm((current) => ({ ...current, [key]: value }))
  }

  const validateServerForm = () => {
    if (!serverForm.externalHost.trim()) return '请输入服务器地址'
    if (!serverForm.uuid.trim()) return '请输入 UUID'
    if (!serverForm.password.trim()) return '请输入统一密码'
    if (!serverForm.realityPrivateKey.trim()) return '请输入 Reality 私钥'
    if (!serverForm.realityPublicKey.trim()) return '请输入 Reality 公钥'
    if (!serverForm.realityShortID.trim()) return '请输入 Reality Short ID'
    if (!serverForm.certificatePath.trim()) return '请输入证书路径'
    if (!serverForm.privateKeyPath.trim()) return '请输入私钥路径'

    const ports = serverPortFields.map(([key]) => serverForm[key])
    if (ports.some((port) => !Number.isInteger(port) || port < 1 || port > 65535)) {
      return '端口必须在 1-65535 之间'
    }
    if (new Set(ports).size !== ports.length) return '五个协议端口不能重复'
    return null
  }

  const handleGenerateServerConfig = async () => {
    const validationError = validateServerForm()
    if (validationError) {
      setError(validationError)
      return
    }

    setGenerating(true)
    setError(null)

    try {
      const response = await axios.post<GenerateResponse>('/api/admin/singbox/config/generate', {
        protocol: 'server',
        options: {
          external_host: serverForm.externalHost.trim(),
          hostname: serverForm.hostname.trim() || serverForm.externalHost.trim(),
          uuid: serverForm.uuid.trim(),
          password: serverForm.password.trim(),
          reality_sni: serverForm.realitySNI.trim() || 'apple.com',
          reality_private_key: serverForm.realityPrivateKey.trim(),
          reality_public_key: serverForm.realityPublicKey.trim(),
          reality_short_id: serverForm.realityShortID.trim(),
          websocket_path: serverForm.websocketPath.trim() || '/vmessws',
          certificate_path: serverForm.certificatePath.trim(),
          private_key_path: serverForm.privateKeyPath.trim(),
          vless_reality_port: serverForm.vlessRealityPort,
          vmess_websocket_port: serverForm.vmessWebSocketPort,
          hysteria2_port: serverForm.hysteria2Port,
          tuic_port: serverForm.tuicPort,
          anytls_port: serverForm.anytlsPort,
        },
      })

      if (response.data.success && response.data.config) {
        const configName = `server-${serverForm.externalHost}-${response.data.port || serverForm.vlessRealityPort}`
        await saveConfig(response.data.config, configName)
        setGeneratedResult({
          protocol: 'server',
          name: configName,
          port: response.data.port || serverForm.vlessRealityPort,
          config: response.data.config,
          links: normalizeLinks(response.data),
        })
        await loadConfigs()
        await loadPortStatus()
        toast.success('服务端配置已生成并保存')
      }
    } catch (err) {
      setError(getErrorMessage(err, '生成服务端配置失败'))
      console.error(err)
    } finally {
      setGenerating(false)
    }
  }

  const handleGenerateClientConfig = async () => {
    if (!serverAddress.trim()) {
      setError('请输入服务器地址')
      return
    }

    setGenerating(true)
    setError(null)

    try {
      const options = {
        server: serverAddress.trim(),
        server_port: autoPort ? 0 : serverPort,
        domain: domain.trim() || serverAddress.trim(),
        path: '/',
        host: domain.trim() || serverAddress.trim(),
      }

      const response = await axios.post<GenerateResponse>('/api/admin/singbox/config/generate', {
        protocol: selectedProtocol,
        options: options,
      })

      if (response.data.success && response.data.config) {
        const configName = `${selectedProtocol}-${serverAddress}-${response.data.port}`
        await saveConfig(response.data.config, configName)
        setGeneratedResult({
          protocol: selectedProtocol,
          name: configName,
          port: response.data.port || serverPort,
          config: response.data.config,
          links: normalizeLinks(response.data),
        })
        await loadConfigs()
        await loadPortStatus()
        setServerAddress('')
        setDomain('')
        setServerPort(443)
        toast.success('单协议配置已生成并保存')
      }
    } catch (err) {
      setError(getErrorMessage(err, '生成配置失败'))
      console.error(err)
    } finally {
      setGenerating(false)
    }
  }

  const saveConfig = async (config: Record<string, unknown>, name: string) => {
    await axios.post('/api/admin/singbox/config/save', {
      name: name,
      config: config,
    })
  }

  const handleDeleteConfig = async (id: number, name: string) => {
    if (!confirm(`确定要删除配置 "${name}" 吗？`)) return

    try {
      await axios.delete(`/api/admin/singbox/config/${id}`)
      await loadConfigs()
      await loadPortStatus()
      toast.success('配置已删除')
    } catch (err) {
      setError(getErrorMessage(err, '删除配置失败'))
      console.error(err)
    }
  }

  const handleAllocatePort = async () => {
    try {
      const response = await axios.post('/api/admin/singbox/port/allocate', {
        count: 1,
        min_port: 10000,
        max_port: 65535,
      })

      if (response.data.ports && response.data.ports.length > 0) {
        const port = response.data.ports[0]
        setServerPort(port)
        setAutoPort(false)
      }
    } catch (err) {
      setError(getErrorMessage(err, '分配端口失败'))
      console.error(err)
    }
  }

  const copyText = async (value: string, label: string) => {
    await navigator.clipboard.writeText(value)
    toast.success(`${label}已复制`)
  }

  if (loading) {
    return (
      <div className='flex h-screen items-center justify-center'>
        <div className='text-lg text-muted-foreground'>加载中...</div>
      </div>
    )
  }

  return (
    <div className='container mx-auto max-w-7xl space-y-6 p-4 md:p-6'>
      <div className='flex flex-col gap-4 md:flex-row md:items-end md:justify-between'>
        <div>
          <div className='flex items-center gap-2 text-sm font-medium text-primary'>
            <Server className='size-4' />
            Sing-box
          </div>
          <h1 className='mt-1 text-2xl font-semibold tracking-normal md:text-3xl'>
            配置管理
          </h1>
          <p className='mt-2 max-w-2xl text-sm text-muted-foreground'>
            服务端五协议配置、单协议配置、分享链接和端口占用集中管理。
          </p>
        </div>

        <div className='inline-flex w-full border border-border bg-card p-1 md:w-auto'>
          <button
            type='button'
            onClick={() => setMode('server')}
            className={cn(
              'flex h-9 flex-1 items-center justify-center gap-2 px-4 text-sm font-medium transition md:flex-none',
              mode === 'server'
                ? 'bg-primary text-primary-foreground'
                : 'text-muted-foreground hover:bg-muted',
            )}
          >
            <Shield className='size-4' />
            服务端
          </button>
          <button
            type='button'
            onClick={() => setMode('client')}
            className={cn(
              'flex h-9 flex-1 items-center justify-center gap-2 px-4 text-sm font-medium transition md:flex-none',
              mode === 'client'
                ? 'bg-primary text-primary-foreground'
                : 'text-muted-foreground hover:bg-muted',
            )}
          >
            <FileJson className='size-4' />
            单协议
          </button>
        </div>
      </div>

      {error && (
        <div className='border border-destructive/40 bg-destructive/10 px-4 py-3 text-sm text-destructive'>
          {error}
        </div>
      )}

      {mode === 'server' ? (
        <div className='pixel-card bg-card p-5 md:p-6'>
          <div className='mb-5 flex items-center justify-between gap-3'>
            <div>
              <h2 className='text-lg font-semibold'>服务端五协议配置</h2>
              <p className='mt-1 text-sm text-muted-foreground'>
                VLESS Reality、VMess WS、Hysteria2、TUIC、AnyTLS。
              </p>
            </div>
            <Button
              type='button'
              onClick={handleGenerateServerConfig}
              disabled={generating}
              className='shrink-0'
            >
              {generating ? <RefreshCw className='size-4 animate-spin' /> : <Save className='size-4' />}
              {generating ? '生成中' : '生成并保存'}
            </Button>
          </div>

          <div className='grid grid-cols-1 gap-5 xl:grid-cols-[1.1fr_0.9fr]'>
            <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
              <TextField
                label='服务器地址'
                value={serverForm.externalHost}
                onChange={(value) => updateServerForm('externalHost', value)}
                placeholder='example.com 或 IP'
              />
              <TextField
                label='主机名'
                value={serverForm.hostname}
                onChange={(value) => updateServerForm('hostname', value)}
                placeholder='默认使用服务器地址'
              />
              <TextField
                label='Reality SNI'
                value={serverForm.realitySNI}
                onChange={(value) => updateServerForm('realitySNI', value)}
              />
              <TextField
                label='WebSocket Path'
                value={serverForm.websocketPath}
                onChange={(value) => updateServerForm('websocketPath', value)}
              />
              <TextField
                label='证书路径'
                value={serverForm.certificatePath}
                onChange={(value) => updateServerForm('certificatePath', value)}
              />
              <TextField
                label='私钥路径'
                value={serverForm.privateKeyPath}
                onChange={(value) => updateServerForm('privateKeyPath', value)}
              />
            </div>

            <div className='grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-1'>
              <SecretField
                label='UUID'
                value={serverForm.uuid}
                onChange={(value) => updateServerForm('uuid', value)}
                onGenerate={() => updateServerForm('uuid', crypto.randomUUID())}
              />
              <SecretField
                label='统一密码'
                value={serverForm.password}
                onChange={(value) => updateServerForm('password', value)}
                onGenerate={() => updateServerForm('password', randomToken())}
              />
              <SecretField
                label='Reality Short ID'
                value={serverForm.realityShortID}
                onChange={(value) => updateServerForm('realityShortID', value)}
                onGenerate={() => updateServerForm('realityShortID', randomHex(4))}
              />
              <TextField
                label='Reality 私钥'
                value={serverForm.realityPrivateKey}
                onChange={(value) => updateServerForm('realityPrivateKey', value)}
                placeholder='private_key'
              />
              <TextField
                label='Reality 公钥'
                value={serverForm.realityPublicKey}
                onChange={(value) => updateServerForm('realityPublicKey', value)}
                placeholder='public_key'
              />
            </div>
          </div>

          <div className='mt-5 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-5'>
            {serverPortFields.map(([key, label]) => (
              <NumberField
                key={key}
                label={label}
                value={serverForm[key]}
                onChange={(value) => updateServerForm(key, value)}
              />
            ))}
          </div>
        </div>
      ) : (
        <div className='pixel-card bg-card p-5 md:p-6'>
          <div className='mb-5 flex items-center justify-between gap-3'>
            <div>
              <h2 className='text-lg font-semibold'>单协议配置</h2>
              <p className='mt-1 text-sm text-muted-foreground'>
                保留原有协议生成入口，适合单节点配置测试。
              </p>
            </div>
            <Button
              type='button'
              onClick={handleGenerateClientConfig}
              disabled={generating || !serverAddress}
              className='shrink-0'
            >
              {generating ? <RefreshCw className='size-4 animate-spin' /> : <Save className='size-4' />}
              {generating ? '生成中' : '生成并保存'}
            </Button>
          </div>

          <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
            <div>
              <label className='mb-2 block text-sm font-medium'>协议类型</label>
              <select
                value={selectedProtocol}
                onChange={(event) => setSelectedProtocol(event.target.value)}
                className='h-10 w-full border border-input bg-background px-3 text-sm outline-none focus:border-ring'
              >
                {singleProtocols.map(([value, label]) => (
                  <option key={value} value={value}>
                    {label}
                  </option>
                ))}
              </select>
            </div>

            <TextField
              label='服务器地址'
              value={serverAddress}
              onChange={setServerAddress}
              placeholder='example.com 或 IP'
            />

            <div>
              <label className='mb-2 block text-sm font-medium'>端口配置</label>
              <div className='flex items-center gap-2'>
                <input
                  type='number'
                  value={serverPort}
                  onChange={(event) => setServerPort(Number(event.target.value))}
                  disabled={autoPort}
                  className='h-10 min-w-0 flex-1 border border-input bg-background px-3 text-sm outline-none disabled:bg-muted disabled:text-muted-foreground'
                  placeholder='端口'
                />
                <Button
                  type='button'
                  variant='outline'
                  size='icon'
                  onClick={handleAllocatePort}
                  title='随机分配端口'
                >
                  <RefreshCw className='size-4' />
                </Button>
                <label className='flex h-10 items-center gap-2 border border-border px-3 text-sm'>
                  <input
                    type='checkbox'
                    checked={autoPort}
                    onChange={(event) => setAutoPort(event.target.checked)}
                  />
                  自动
                </label>
              </div>
            </div>

            <TextField
              label='域名'
              value={domain}
              onChange={setDomain}
              placeholder='默认使用服务器地址'
            />
          </div>
        </div>
      )}

      {generatedResult && (
        <div className='grid grid-cols-1 gap-6 xl:grid-cols-[0.9fr_1.1fr]'>
          <div className='pixel-card bg-card p-5'>
            <div className='mb-4 flex items-center justify-between gap-3'>
              <div>
                <div className='flex items-center gap-2 text-sm font-medium text-primary'>
                  <CheckCircle2 className='size-4' />
                  {generatedResult.name}
                </div>
                <h2 className='mt-1 text-lg font-semibold'>分享链接</h2>
              </div>
              <Button
                type='button'
                variant='outline'
                size='sm'
                onClick={() =>
                  copyText(Object.values(generatedResult.links).join('\n'), '分享链接')
                }
                disabled={Object.keys(generatedResult.links).length === 0}
              >
                <Copy className='size-4' />
                全部复制
              </Button>
            </div>

            {Object.keys(generatedResult.links).length === 0 ? (
              <div className='border border-border bg-muted/35 px-4 py-6 text-center text-sm text-muted-foreground'>
                当前响应没有返回分享链接。
              </div>
            ) : (
              <div className='space-y-3'>
                {Object.entries(generatedResult.links).map(([protocol, link]) => (
                  <div key={protocol} className='border border-border bg-background p-3'>
                    <div className='mb-2 flex items-center justify-between gap-2'>
                      <span className='inline-flex items-center gap-2 text-sm font-medium uppercase'>
                        <Link2 className='size-4 text-primary' />
                        {protocol}
                      </span>
                      <Button
                        type='button'
                        variant='ghost'
                        size='sm'
                        onClick={() => copyText(link, `${protocol} 链接`)}
                      >
                        <Copy className='size-4' />
                        复制
                      </Button>
                    </div>
                    <div className='break-all font-mono text-xs leading-5 text-muted-foreground'>
                      {link}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>

          <div className='pixel-card bg-card p-5'>
            <div className='mb-4 flex items-center justify-between gap-3'>
              <div>
                <div className='flex items-center gap-2 text-sm font-medium text-primary'>
                  <FileJson className='size-4' />
                  端口 {generatedResult.port}
                </div>
                <h2 className='mt-1 text-lg font-semibold'>配置 JSON</h2>
              </div>
              <Button
                type='button'
                variant='outline'
                size='sm'
                onClick={() => copyText(generatedJSON, '配置 JSON')}
              >
                <Copy className='size-4' />
                复制
              </Button>
            </div>
            <textarea
              readOnly
              value={generatedJSON}
              className='h-80 w-full resize-y border border-input bg-background p-3 font-mono text-xs leading-5 outline-none'
            />
          </div>
        </div>
      )}

      {portStatus && (
        <div className='grid grid-cols-1 gap-4 md:grid-cols-3'>
          <MetricCard label='已用端口' value={String(portStatus.used_ports.length)} tone='blue' />
          <MetricCard label='可用端口' value={String(portStatus.available_count)} tone='green' />
          <MetricCard
            label='端口范围'
            value={`${portStatus.range.min}-${portStatus.range.max}`}
            tone='neutral'
          />
        </div>
      )}

      <div className='pixel-card bg-card p-5 md:p-6'>
        <div className='mb-4 flex items-center justify-between gap-3'>
          <h2 className='text-lg font-semibold'>已保存配置</h2>
          <Button type='button' variant='outline' size='sm' onClick={loadConfigs}>
            <RefreshCw className='size-4' />
            刷新
          </Button>
        </div>

        {configs.length === 0 ? (
          <div className='border border-border bg-muted/35 py-8 text-center text-sm text-muted-foreground'>
            暂无保存的配置
          </div>
        ) : (
          <div className='overflow-x-auto'>
            <table className='min-w-full divide-y divide-border'>
              <thead className='bg-muted/50'>
                <tr>
                  <TableHead>名称</TableHead>
                  <TableHead>协议</TableHead>
                  <TableHead>端口</TableHead>
                  <TableHead>状态</TableHead>
                  <TableHead>创建时间</TableHead>
                  <TableHead>操作</TableHead>
                </tr>
              </thead>
              <tbody className='divide-y divide-border bg-card'>
                {configs.map((config) => (
                  <tr key={config.id} className='hover:bg-muted/35'>
                    <td className='max-w-[260px] truncate px-4 py-3 text-sm font-medium'>
                      {config.name}
                    </td>
                    <td className='px-4 py-3'>
                      <span className='inline-flex border border-primary/25 bg-primary/10 px-2 py-1 text-xs font-medium text-primary'>
                        {config.protocol || 'config'}
                      </span>
                    </td>
                    <td className='px-4 py-3 font-mono text-sm'>{config.port || '-'}</td>
                    <td className='px-4 py-3'>
                      <span
                        className={cn(
                          'inline-flex border px-2 py-1 text-xs font-medium',
                          config.enabled
                            ? 'border-emerald-500/25 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
                            : 'border-destructive/25 bg-destructive/10 text-destructive',
                        )}
                      >
                        {config.enabled ? '启用' : '禁用'}
                      </span>
                    </td>
                    <td className='whitespace-nowrap px-4 py-3 text-sm text-muted-foreground'>
                      {new Date(config.created_at).toLocaleString()}
                    </td>
                    <td className='px-4 py-3'>
                      <Button
                        type='button'
                        variant='ghost'
                        size='sm'
                        onClick={() => handleDeleteConfig(config.id, config.name)}
                        className='text-destructive hover:text-destructive'
                      >
                        <Trash2 className='size-4' />
                        删除
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}

function TextField({
  label,
  value,
  onChange,
  placeholder,
}: {
  label: string
  value: string
  onChange: (value: string) => void
  placeholder?: string
}) {
  return (
    <div>
      <label className='mb-2 block text-sm font-medium'>{label}</label>
      <input
        type='text'
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
        className='h-10 w-full border border-input bg-background px-3 text-sm outline-none placeholder:text-muted-foreground focus:border-ring'
      />
    </div>
  )
}

function SecretField({
  label,
  value,
  onChange,
  onGenerate,
}: {
  label: string
  value: string
  onChange: (value: string) => void
  onGenerate: () => void
}) {
  return (
    <div>
      <label className='mb-2 flex items-center gap-2 text-sm font-medium'>
        <KeyRound className='size-4 text-primary' />
        {label}
      </label>
      <div className='flex items-center gap-2'>
        <input
          type='text'
          value={value}
          onChange={(event) => onChange(event.target.value)}
          className='h-10 min-w-0 flex-1 border border-input bg-background px-3 font-mono text-sm outline-none focus:border-ring'
        />
        <Button type='button' variant='outline' size='icon' onClick={onGenerate} title='重新生成'>
          <RefreshCw className='size-4' />
        </Button>
      </div>
    </div>
  )
}

function NumberField({
  label,
  value,
  onChange,
}: {
  label: string
  value: number
  onChange: (value: number) => void
}) {
  return (
    <div>
      <label className='mb-2 block truncate text-sm font-medium'>{label}</label>
      <input
        type='number'
        min={1}
        max={65535}
        value={value}
        onChange={(event) => onChange(Number(event.target.value))}
        className='h-10 w-full border border-input bg-background px-3 font-mono text-sm outline-none focus:border-ring'
      />
    </div>
  )
}

function MetricCard({
  label,
  value,
  tone,
}: {
  label: string
  value: string
  tone: 'blue' | 'green' | 'neutral'
}) {
  return (
    <div
      className={cn(
        'pixel-card bg-card p-4',
        tone === 'blue' && 'border-sky-500/30',
        tone === 'green' && 'border-emerald-500/30',
      )}
    >
      <div className='text-sm text-muted-foreground'>{label}</div>
      <div className='mt-1 break-all font-mono text-2xl font-semibold'>{value}</div>
    </div>
  )
}

function TableHead({ children }: { children: ReactNode }) {
  return (
    <th className='whitespace-nowrap px-4 py-3 text-left text-xs font-medium uppercase text-muted-foreground'>
      {children}
    </th>
  )
}
