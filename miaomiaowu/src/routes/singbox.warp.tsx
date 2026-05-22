import { useState, useEffect } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { api } from '@/lib/api'

interface WARPConfig {
  id: string
  name: string
  type: string
  account_id: string
  license_key: string
  enabled: boolean
  port: number
  preferred_server: boolean
  created_at: string
  status: {
    enabled: boolean
    type: string
    account_id?: string
    connected: boolean
    preferred_server: boolean
    last_updated: string
  }
}

export const Route = createFileRoute('/singbox/warp')({
  component: WARPPage,
})

function WARPPage() {
  const [configs, setConfigs] = useState<WARPConfig[]>([])
  const [warpStatus, setWarpStatus] = useState<any>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [successMessage, setSuccessMessage] = useState<string | null>(null)

  // 表单状态
  const [showEnableForm, setShowEnableForm] = useState(false)
  const [formData, setFormData] = useState({
    type: 'warp',
    license_key: '',
    port: 0,
    preferred_server: false,
  })

  useEffect(() => {
    loadData()
    const interval = setInterval(loadData, 10000)
    return () => clearInterval(interval)
  }, [])

  const loadData = async () => {
    try {
      const [configsRes, statusRes] = await Promise.all([
        api.get('/api/admin/warp/configs'),
        api.get('/api/admin/warp/status'),
      ])

      setConfigs(configsRes.data.configs || [])
      setWarpStatus(statusRes.data)
      setError(null)
    } catch (err) {
      console.error('加载WARP数据失败:', err)
      setError('加载WARP数据失败')
    } finally {
      setLoading(false)
    }
  }

  const handleEnableWARP = async () => {
    setLoading(true)
    setError(null)

    try {
      const response = await api.post('/api/admin/warp/enable', {
        type: formData.type,
        license_key: formData.license_key,
        port: formData.port,
        preferred_server: formData.preferred_server,
      })

      if (response.data.status === 'success') {
        setSuccessMessage('WARP启用成功')
        setShowEnableForm(false)
        setFormData({
          type: 'warp',
          license_key: '',
          port: 0,
          preferred_server: false,
        })
        await loadData()
      }
    } catch (err) {
      setError('启用WARP失败')
      console.error(err)
    } finally {
      setLoading(false)
    }
  }

  const handleDisableWARP = async () => {
    if (!confirm('确定要禁用WARP吗？')) return

    try {
      await api.post('/api/admin/warp/disable')

      setSuccessMessage('WARP已禁用')
      await loadData()
    } catch (err) {
      setError('禁用WARP失败')
      console.error(err)
    }
  }

  const handleCheckConnection = async () => {
    try {
      const response = await api.get('/api/admin/warp/check-connection')
      const { connected, ip_address, message } = response.data

      if (connected) {
        setSuccessMessage(`WARP连接正常 - IP: ${ip_address}`)
      } else {
        setError(message || 'WARP未连接')
      }
    } catch (err) {
      setError('检查连接失败')
      console.error(err)
    }
  }

  const handleDeleteConfig = async (configId: string) => {
    if (!confirm('确定要删除此配置吗？')) return

    try {
      await api.delete('/api/admin/warp/delete', {
        params: { config_id: configId },
      })

      setSuccessMessage('配置删除成功')
      await loadData()
    } catch (err) {
      setError('删除配置失败')
      console.error(err)
    }
  }

  const getWARPTypeLabel = (type: string) => {
    const labels: Record<string, string> = {
      warp: '官方WARP',
      warpo: 'WARP-GO',
    }
    return labels[type] || type
  }

  const getWARPTypeColor = (type: string) => {
    const colors: Record<string, string> = {
      warp: 'bg-blue-100 text-blue-800',
      warpo: 'bg-purple-100 text-purple-800',
    }
    return colors[type] || 'bg-gray-100 text-gray-800'
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
        <h1 className='text-3xl font-bold'>WARP管理</h1>
        <p className='mt-2 text-gray-600'>管理Cloudflare WARP代理</p>
      </div>

      {error && (
        <div className='mb-4 rounded border border-red-400 bg-red-100 px-4 py-3 text-red-700'>
          {error}
        </div>
      )}

      {successMessage && (
        <div className='mb-4 rounded border border-green-400 bg-green-100 px-4 py-3 text-green-700'>
          {successMessage}
        </div>
      )}

      {/* WARP状态卡片 */}
      {warpStatus && (
        <div className='mb-6 rounded-lg bg-white p-6 shadow'>
          <h2 className='mb-4 text-xl font-semibold'>WARP状态</h2>

          <div className='grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4'>
            <div className='rounded bg-gray-50 p-4'>
              <div className='text-sm text-gray-600'>状态</div>
              <div className='text-lg font-semibold'>
                {warpStatus.enabled ? (
                  <span className='text-green-600'>已启用</span>
                ) : (
                  <span className='text-red-600'>未启用</span>
                )}
              </div>
            </div>

            <div className='rounded bg-gray-50 p-4'>
              <div className='text-sm text-gray-600'>类型</div>
              <div className='text-lg font-semibold'>
                {getWARPTypeLabel(warpStatus.type)}
              </div>
            </div>

            <div className='rounded bg-gray-50 p-4'>
              <div className='text-sm text-gray-600'>连接状态</div>
              <div className='text-lg font-semibold'>
                {warpStatus.connected ? (
                  <span className='text-green-600'>已连接</span>
                ) : (
                  <span className='text-red-600'>未连接</span>
                )}
              </div>
            </div>

            <div className='rounded bg-gray-50 p-4'>
              <div className='text-sm text-gray-600'>IP类型</div>
              <div className='text-lg font-semibold'>
                {warpStatus.ip_address_type || '未知'}
              </div>
            </div>
          </div>

          <div className='mt-4 flex space-x-4'>
            <button
              onClick={handleCheckConnection}
              className='rounded bg-blue-500 px-4 py-2 text-white hover:bg-blue-600'
            >
              检查连接
            </button>

            {warpStatus.enabled && (
              <button
                onClick={handleDisableWARP}
                className='rounded bg-red-500 px-4 py-2 text-white hover:bg-red-600'
              >
                禁用WARP
              </button>
            )}
          </div>
        </div>
      )}

      {/* 启用WARP表单 */}
      <div className='mb-6 rounded-lg bg-white p-6 shadow'>
        <h2 className='mb-4 text-xl font-semibold'>
          {!warpStatus?.enabled ? '启用WARP' : '重新配置WARP'}
        </h2>

        {!showEnableForm ? (
          <button
            onClick={() => setShowEnableForm(true)}
            className='rounded bg-blue-500 px-4 py-2 text-white hover:bg-blue-600'
          >
            {warpStatus?.enabled ? '重新配置' : '启用WARP'}
          </button>
        ) : (
          <div>
            <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
              <div>
                <label className='mb-2 block text-sm font-medium text-gray-700'>
                  WARP类型
                </label>
                <select
                  value={formData.type}
                  onChange={(e) =>
                    setFormData({ ...formData, type: e.target.value })
                  }
                  className='w-full rounded-md border border-gray-300 px-3 py-2'
                >
                  <option value='warp'>官方WARP</option>
                  <option value='warpo'>WARP-GO</option>
                </select>
              </div>

              <div>
                <label className='mb-2 block text-sm font-medium text-gray-700'>
                  License Key
                </label>
                <input
                  type='text'
                  value={formData.license_key}
                  onChange={(e) =>
                    setFormData({ ...formData, license_key: e.target.value })
                  }
                  placeholder='可选，用于WARP+账户'
                  className='w-full rounded-md border border-gray-300 px-3 py-2'
                />
              </div>

              {formData.type === 'warpo' && (
                <>
                  <div>
                    <label className='mb-2 block text-sm font-medium text-gray-700'>
                      端口
                    </label>
                    <input
                      type='number'
                      value={formData.port || 40000}
                      onChange={(e) =>
                        setFormData({
                          ...formData,
                          port: parseInt(e.target.value),
                        })
                      }
                      placeholder='40000'
                      className='w-full rounded-md border border-gray-300 px-3 py-2'
                    />
                  </div>

                  <div className='flex items-center'>
                    <input
                      type='checkbox'
                      checked={formData.preferred_server}
                      onChange={(e) =>
                        setFormData({
                          ...formData,
                          preferred_server: e.target.checked,
                        })
                      }
                      className='h-4 w-4 rounded text-blue-600'
                    />
                    <label className='ml-2 text-sm font-medium text-gray-700'>
                      使用优选服务器
                    </label>
                  </div>
                </>
              )}
            </div>

            <div className='mt-4 flex space-x-4'>
              <button
                onClick={handleEnableWARP}
                disabled={loading}
                className='rounded bg-green-500 px-6 py-2 text-white hover:bg-green-600 disabled:opacity-50'
              >
                {loading ? '处理中...' : '启用'}
              </button>

              <button
                onClick={() => {
                  setShowEnableForm(false)
                  setFormData({
                    type: 'warp',
                    license_key: '',
                    port: 0,
                    preferred_server: false,
                  })
                }}
                className='rounded bg-gray-500 px-6 py-2 text-white hover:bg-gray-600'
              >
                取消
              </button>
            </div>
          </div>
        )}
      </div>

      {/* WARP配置列表 */}
      <div className='rounded-lg bg-white p-6 shadow'>
        <h2 className='mb-4 text-xl font-semibold'>WARP配置列表</h2>

        {configs.length === 0 ? (
          <div className='py-8 text-center text-gray-500'>暂无WARP配置</div>
        ) : (
          <div className='overflow-x-auto'>
            <table className='min-w-full divide-y divide-gray-200'>
              <thead className='bg-gray-50'>
                <tr>
                  <th className='px-6 py-3 text-left text-xs font-medium tracking-wider text-gray-500 uppercase'>
                    名称
                  </th>
                  <th className='px-6 py-3 text-left text-xs font-medium tracking-wider text-gray-500 uppercase'>
                    类型
                  </th>
                  <th className='px-6 py-3 text-left text-xs font-medium tracking-wider text-gray-500 uppercase'>
                    状态
                  </th>
                  <th className='px-6 py-3 text-left text-xs font-medium tracking-wider text-gray-500 uppercase'>
                    账户ID
                  </th>
                  <th className='px-6 py-3 text-left text-xs font-medium tracking-wider text-gray-500 uppercase'>
                    创建时间
                  </th>
                  <th className='px-6 py-3 text-left text-xs font-medium tracking-wider text-gray-500 uppercase'>
                    操作
                  </th>
                </tr>
              </thead>
              <tbody className='divide-y divide-gray-200 bg-white'>
                {configs.map((config) => (
                  <tr key={config.id}>
                    <td className='px-6 py-4 whitespace-nowrap'>
                      {config.name}
                    </td>
                    <td className='px-6 py-4 whitespace-nowrap'>
                      <span
                        className={`inline-flex rounded-full px-2 text-xs leading-5 font-semibold ${getWARPTypeColor(config.type)}`}
                      >
                        {getWARPTypeLabel(config.type)}
                      </span>
                    </td>
                    <td className='px-6 py-4 whitespace-nowrap'>
                      <span
                        className={`inline-flex rounded-full px-2 text-xs leading-5 font-semibold ${
                          config.status.enabled
                            ? 'bg-green-100 text-green-800'
                            : 'bg-gray-100 text-gray-800'
                        }`}
                      >
                        {config.status.enabled ? '已启用' : '未启用'}
                      </span>
                    </td>
                    <td className='px-6 py-4 text-sm whitespace-nowrap text-gray-500'>
                      {config.status.account_id || '-'}
                    </td>
                    <td className='px-6 py-4 text-sm whitespace-nowrap text-gray-500'>
                      {new Date(config.created_at).toLocaleString()}
                    </td>
                    <td className='px-6 py-4 text-sm font-medium whitespace-nowrap'>
                      <button
                        onClick={() => handleDeleteConfig(config.id)}
                        className='text-red-600 hover:text-red-900'
                      >
                        删除
                      </button>
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
