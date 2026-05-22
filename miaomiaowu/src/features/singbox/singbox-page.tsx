import { useState, useEffect } from 'react'
import { Link } from '@tanstack/react-router'
import { api } from '@/lib/api'

interface SingboxStatus {
  installed: boolean
  version: string
  environment: string
  system_info: {
    os: string
    arch: string
    kernel: string
    hostname: string
    environment: string
    capabilities: string[]
  }
}

interface ServiceStatus {
  running: boolean
  enabled: boolean
  pid: number
  memory: string
  uptime: string
  version: string
  last_restart: string
}

export function SingboxPage() {
  const [status, setStatus] = useState<SingboxStatus | null>(null)
  const [serviceStatus, setServiceStatus] = useState<ServiceStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [installing, setInstalling] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    loadStatus()
    loadServiceStatus()
    // 定期刷新服务状态
    const interval = setInterval(loadServiceStatus, 5000)
    return () => clearInterval(interval)
  }, [])

  const loadStatus = async () => {
    try {
      const response = await api.get('/api/admin/singbox/install-status')
      setStatus(response.data)
    } catch (err) {
      setError('加载状态失败')
      console.error(err)
    } finally {
      setLoading(false)
    }
  }

  const loadServiceStatus = async () => {
    try {
      const response = await api.get('/api/admin/singbox/service/status')
      setServiceStatus(response.data)
    } catch (err) {
      console.error('加载服务状态失败:', err)
    }
  }

  const handleInstall = async () => {
    setInstalling(true)
    setError(null)
    try {
      const response = await api.post('/api/admin/singbox/install', {
        version: '',
      })
      if (response.data.status === 'success') {
        await loadStatus()
        await loadServiceStatus()
      }
    } catch (err) {
      setError('安装失败')
      console.error(err)
    } finally {
      setInstalling(false)
    }
  }

  const handleUninstall = async () => {
    if (!confirm('确定要卸载 Sing-box 吗？')) return

    setInstalling(true)
    setError(null)
    try {
      const response = await api.post('/api/admin/singbox/uninstall')
      if (response.data.status === 'success') {
        await loadStatus()
        await loadServiceStatus()
      }
    } catch (err) {
      setError('卸载失败')
      console.error(err)
    } finally {
      setInstalling(false)
    }
  }

  const handleServiceAction = async (
    action: 'start' | 'stop' | 'restart' | 'enable' | 'disable'
  ) => {
    setError(null)
    try {
      const response = await api.post(`/api/admin/singbox/service/${action}`)
      if (response.data.status === 'success') {
        await loadServiceStatus()
      }
    } catch (err) {
      setError(`${action} 操作失败`)
      console.error(err)
    }
  }

  if (loading) {
    return (
      <div className='flex h-screen items-center justify-center'>
        <div className='text-lg'>加载中...</div>
      </div>
    )
  }

  return (
    <div className='container mx-auto p-6'>
      <div className='mb-6'>
        <h1 className='text-3xl font-bold'>Sing-box 管理</h1>
        <p className='mt-2 text-gray-600'>管理 Sing-box 代理服务的安装和运行</p>

        {/* 导航链接 */}
        <div className='mt-4 flex space-x-4'>
          <Link
            to='/singbox'
            className='font-medium text-blue-600 hover:text-blue-800'
          >
            服务管理
          </Link>
          <Link
            to='/singbox/config'
            className='font-medium text-blue-600 hover:text-blue-800'
          >
            配置管理
          </Link>
        </div>
      </div>

      {error && (
        <div className='mb-4 rounded border border-red-400 bg-red-100 px-4 py-3 text-red-700'>
          {error}
        </div>
      )}

      {/* 安装状态 */}
      <div className='mb-6 rounded-lg bg-white p-6 shadow'>
        <h2 className='mb-4 text-xl font-semibold'>安装状态</h2>
        {status ? (
          <div className='space-y-2'>
            <div className='flex items-center'>
              <span className='w-32 text-gray-600'>安装状态:</span>
              <span
                className={`font-medium ${status.installed ? 'text-green-600' : 'text-red-600'}`}
              >
                {status.installed ? '已安装' : '未安装'}
              </span>
            </div>
            {status.installed && (
              <div className='flex items-center'>
                <span className='w-32 text-gray-600'>版本:</span>
                <span className='font-medium'>{status.version}</span>
              </div>
            )}
            <div className='flex items-center'>
              <span className='w-32 text-gray-600'>环境:</span>
              <span className='font-medium'>{status.environment}</span>
            </div>
          </div>
        ) : (
          <div className='text-gray-500'>无法获取安装状态</div>
        )}

        <div className='mt-4 space-x-2'>
          {!status?.installed && (
            <button
              onClick={handleInstall}
              disabled={installing}
              className='rounded bg-blue-500 px-4 py-2 text-white hover:bg-blue-600 disabled:opacity-50'
            >
              {installing ? '安装中...' : '安装 Sing-box'}
            </button>
          )}
          {status?.installed && (
            <button
              onClick={handleUninstall}
              disabled={installing}
              className='rounded bg-red-500 px-4 py-2 text-white hover:bg-red-600 disabled:opacity-50'
            >
              {installing ? '卸载中...' : '卸载 Sing-box'}
            </button>
          )}
        </div>
      </div>

      {/* 服务状态 */}
      {status?.installed && (
        <div className='mb-6 rounded-lg bg-white p-6 shadow'>
          <h2 className='mb-4 text-xl font-semibold'>服务状态</h2>
          {serviceStatus ? (
            <div className='space-y-2'>
              <div className='flex items-center'>
                <span className='w-32 text-gray-600'>运行状态:</span>
                <span
                  className={`font-medium ${serviceStatus.running ? 'text-green-600' : 'text-red-600'}`}
                >
                  {serviceStatus.running ? '运行中' : '已停止'}
                </span>
              </div>
              <div className='flex items-center'>
                <span className='w-32 text-gray-600'>开机自启:</span>
                <span
                  className={`font-medium ${serviceStatus.enabled ? 'text-green-600' : 'text-gray-600'}`}
                >
                  {serviceStatus.enabled ? '已启用' : '未启用'}
                </span>
              </div>
              {serviceStatus.running && serviceStatus.pid > 0 && (
                <div className='flex items-center'>
                  <span className='w-32 text-gray-600'>进程ID:</span>
                  <span className='font-medium'>{serviceStatus.pid}</span>
                </div>
              )}
              {serviceStatus.version && (
                <div className='flex items-center'>
                  <span className='w-32 text-gray-600'>版本:</span>
                  <span className='font-medium'>{serviceStatus.version}</span>
                </div>
              )}
            </div>
          ) : (
            <div className='text-gray-500'>无法获取服务状态</div>
          )}

          <div className='mt-4 space-x-2'>
            {serviceStatus?.running ? (
              <>
                <button
                  onClick={() => handleServiceAction('stop')}
                  className='rounded bg-yellow-500 px-4 py-2 text-white hover:bg-yellow-600'
                >
                  停止服务
                </button>
                <button
                  onClick={() => handleServiceAction('restart')}
                  className='rounded bg-blue-500 px-4 py-2 text-white hover:bg-blue-600'
                >
                  重启服务
                </button>
              </>
            ) : (
              <button
                onClick={() => handleServiceAction('start')}
                className='rounded bg-green-500 px-4 py-2 text-white hover:bg-green-600'
              >
                启动服务
              </button>
            )}
            {serviceStatus?.enabled ? (
              <button
                onClick={() => handleServiceAction('disable')}
                className='rounded bg-gray-500 px-4 py-2 text-white hover:bg-gray-600'
              >
                禁用开机自启
              </button>
            ) : (
              <button
                onClick={() => handleServiceAction('enable')}
                className='rounded bg-purple-500 px-4 py-2 text-white hover:bg-purple-600'
              >
                启用开机自启
              </button>
            )}
          </div>
        </div>
      )}

      {/* 系统信息 */}
      {status?.system_info && (
        <div className='rounded-lg bg-white p-6 shadow'>
          <h2 className='mb-4 text-xl font-semibold'>系统信息</h2>
          <div className='space-y-2'>
            <div className='flex items-center'>
              <span className='w-32 text-gray-600'>操作系统:</span>
              <span className='font-medium'>{status.system_info.os}</span>
            </div>
            <div className='flex items-center'>
              <span className='w-32 text-gray-600'>架构:</span>
              <span className='font-medium'>{status.system_info.arch}</span>
            </div>
            <div className='flex items-center'>
              <span className='w-32 text-gray-600'>主机名:</span>
              <span className='font-medium'>{status.system_info.hostname}</span>
            </div>
            {status.system_info.capabilities.length > 0 && (
              <div className='flex items-center'>
                <span className='w-32 text-gray-600'>系统能力:</span>
                <div className='flex flex-wrap gap-2'>
                  {status.system_info.capabilities.map((cap, index) => (
                    <span
                      key={index}
                      className='rounded bg-blue-100 px-2 py-1 text-sm text-blue-800'
                    >
                      {cap}
                    </span>
                  ))}
                </div>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
