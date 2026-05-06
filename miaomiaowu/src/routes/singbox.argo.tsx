import { createFileRoute } from '@tanstack/react-router'
import { useState, useEffect } from 'react'
import axios from 'axios'

interface ArgoTunnel {
  id: string
  name: string
  type: string
  domain: string
  enabled: boolean
  port: number
  local_service: string
  created_at: string
  last_used: string
  status: {
    running: boolean
    url: string
    error: string
    connected: boolean
  }
}

export const Route = createFileRoute('/singbox/argo')({
  component: ArgoTunnelsPage,
})

function ArgoTunnelsPage() {
  const [tunnels, setTunnels] = useState<ArgoTunnel[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [successMessage, setSuccessMessage] = useState<string | null>(null)

  // 表单状态
  const [showCreateForm, setShowCreateForm] = useState(false)
  const [formData, setFormData] = useState({
    name: '',
    type: 'temp',
    domain: '',
    token: '',
    local_port: 8080,
  })

  useEffect(() => {
    loadTunnels()
    // 自动刷新状态
    const interval = setInterval(loadTunnels, 5000)
    return () => clearInterval(interval)
  }, [])

  const loadTunnels = async () => {
    try {
      const response = await axios.get('/api/admin/argo/list')
      setTunnels(response.data.tunnels || [])
      setError(null)
    } catch (err) {
      console.error('加载隧道列表失败:', err)
      setError('加载隧道列表失败')
    } finally {
      setLoading(false)
    }
  }

  const handleCreateTunnel = async () => {
    if (!formData.name) {
      setError('请输入隧道名称')
      return
    }

    setLoading(true)
    setError(null)

    try {
      const response = await axios.post('/api/admin/argo/create', {
        name: formData.name,
        type: formData.type,
        domain: formData.domain,
        token: formData.token,
        local_port: formData.local_port,
      })

      if (response.data.status === 'success') {
        setSuccessMessage('隧道创建成功')
        setShowCreateForm(false)
        setFormData({
          name: '',
          type: 'temp',
          domain: '',
          token: '',
          local_port: 8080,
        })
        await loadTunnels()
      }
    } catch (err) {
      setError('创建隧道失败')
      console.error(err)
    } finally {
      setLoading(false)
    }
  }

  const handleTunnelAction = async (tunnelId: string, action: string) => {
    try {
      await axios.post('/api/admin/argo/action', {
        tunnel_id: tunnelId,
        action: action,
      })

      setSuccessMessage(`隧道${action === 'start' ? '启动' : action === 'stop' ? '停止' : '删除'}成功`)
      await loadTunnels()
    } catch (err) {
      setError(`隧道${action}失败`)
      console.error(err)
    }
  }

  const handleQuickTunnel = async () => {
    if (!formData.name) {
      setError('请输入隧道名称')
      return
    }

    setLoading(true)
    setError(null)

    try {
      const response = await axios.post('/api/admin/argo/quick', {
        name: formData.name,
        local_port: formData.local_port,
      })

      if (response.data.status === 'success') {
        setSuccessMessage(`快速隧道创建成功: ${response.data.url}`)
        setShowCreateForm(false)
        await loadTunnels()
      }
    } catch (err) {
      setError('创建快速隧道失败')
      console.error(err)
    } finally {
      setLoading(false)
    }
  }

  const getTunnelTypeLabel = (type: string) => {
    const labels: Record<string, string> = {
      'fixed': '固定域名',
      'temp': '临时隧道',
      'argogo': 'Argo-Go',
    }
    return labels[type] || type
  }

  const getTunnelTypeColor = (type: string) => {
    const colors: Record<string, string> = {
      'fixed': 'bg-purple-100 text-purple-800',
      'temp': 'bg-blue-100 text-blue-800',
      'argogo': 'bg-green-100 text-green-800',
    }
    return colors[type] || 'bg-gray-100 text-gray-800'
  }

  if (loading && tunnels.length === 0) {
    return (
      <div className="flex items-center justify-center h-screen">
        <div className="text-lg">加载中...</div>
      </div>
    )
  }

  return (
    <div className="container mx-auto p-6">
      <div className="mb-6">
        <h1 className="text-3xl font-bold">Argo隧道管理</h1>
        <p className="text-gray-600 mt-2">管理Cloudflare Argo隧道</p>
      </div>

      {error && (
        <div className="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded mb-4">
          {error}
        </div>
      )}

      {successMessage && (
        <div className="bg-green-100 border border-green-400 text-green-700 px-4 py-3 rounded mb-4">
          {successMessage}
        </div>
      )}

      {/* 操作按钮 */}
      <div className="bg-white rounded-lg shadow p-6 mb-6">
        <h2 className="text-xl font-semibold mb-4">隧道操作</h2>

        <div className="flex space-x-4">
          <button
            onClick={() => setShowCreateForm(true)}
            className="bg-blue-500 text-white px-4 py-2 rounded hover:bg-blue-600"
          >
            创建隧道
          </button>

          <button
            onClick={() => {
              setShowCreateForm(true)
              setFormData({ ...formData, type: 'quick' })
            }}
            className="bg-green-500 text-white px-4 py-2 rounded hover:bg-green-600"
          >
            快速隧道
          </button>
        </div>
      </div>

      {/* 创建隧道表单 */}
      {showCreateForm && (
        <div className="bg-white rounded-lg shadow p-6 mb-6">
          <h2 className="text-xl font-semibold mb-4">
            {formData.type === 'quick' ? '创建快速隧道' : '创建隧道'}
          </h2>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">
                隧道名称 *
              </label>
              <input
                type="text"
                value={formData.name}
                onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                placeholder="my-tunnel"
                className="w-full border border-gray-300 rounded-md px-3 py-2"
              />
            </div>

            {formData.type !== 'quick' && (
              <>
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-2">
                    隧道类型
                  </label>
                  <select
                    value={formData.type}
                    onChange={(e) => setFormData({ ...formData, type: e.target.value })}
                    className="w-full border border-gray-300 rounded-md px-3 py-2"
                  >
                    <option value="temp">临时隧道</option>
                    <option value="fixed">固定域名</option>
                    <option value="argogo">Argo-Go</option>
                  </select>
                </div>

                {formData.type === 'fixed' && (
                  <>
                    <div>
                      <label className="block text-sm font-medium text-gray-700 mb-2">
                        域名 *
                      </label>
                      <input
                        type="text"
                        value={formData.domain}
                        onChange={(e) => setFormData({ ...formData, domain: e.target.value })}
                        placeholder="tunnel.example.com"
                        className="w-full border border-gray-300 rounded-md px-3 py-2"
                      />
                    </div>

                    <div>
                      <label className="block text-sm font-medium text-gray-700 mb-2">
                        Token *
                      </label>
                      <input
                        type="text"
                        value={formData.token}
                        onChange={(e) => setFormData({ ...formData, token: e.target.value })}
                        placeholder="Cloudflare Token"
                        className="w-full border border-gray-300 rounded-md px-3 py-2"
                      />
                    </div>
                  </>
                )}
              </>
            )}

            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">
                本地端口 *
              </label>
              <input
                type="number"
                value={formData.local_port}
                onChange={(e) => setFormData({ ...formData, local_port: parseInt(e.target.value) })}
                placeholder="8080"
                className="w-full border border-gray-300 rounded-md px-3 py-2"
              />
            </div>
          </div>

          <div className="mt-4 flex space-x-4">
            {formData.type === 'quick' ? (
              <button
                onClick={handleQuickTunnel}
                disabled={loading}
                className="bg-green-500 text-white px-6 py-2 rounded hover:bg-green-600 disabled:opacity-50"
              >
                {loading ? '创建中...' : '创建快速隧道'}
              </button>
            ) : (
              <button
                onClick={handleCreateTunnel}
                disabled={loading}
                className="bg-blue-500 text-white px-6 py-2 rounded hover:bg-blue-600 disabled:opacity-50"
              >
                {loading ? '创建中...' : '创建隧道'}
              </button>
            )}

            <button
              onClick={() => {
                setShowCreateForm(false)
                setFormData({
                  name: '',
                  type: 'temp',
                  domain: '',
                  token: '',
                  local_port: 8080,
                })
              }}
              className="bg-gray-500 text-white px-6 py-2 rounded hover:bg-gray-600"
            >
              取消
            </button>
          </div>
        </div>
      )}

      {/* 隧道列表 */}
      <div className="bg-white rounded-lg shadow p-6">
        <h2 className="text-xl font-semibold mb-4">
          隧道列表 ({tunnels.length})
        </h2>

        {tunnels.length === 0 ? (
          <div className="text-center py-8 text-gray-500">
            暂无隧道，点击上方按钮创建
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="min-w-full divide-y divide-gray-200">
              <thead className="bg-gray-50">
                <tr>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    名称
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    类型
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    域名
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    状态
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    URL
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    操作
                  </th>
                </tr>
              </thead>
              <tbody className="bg-white divide-y divide-gray-200">
                {tunnels.map((tunnel) => (
                  <tr key={tunnel.id}>
                    <td className="px-6 py-4 whitespace-nowrap">{tunnel.name}</td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      <span className={`px-2 inline-flex text-xs leading-5 font-semibold rounded-full ${getTunnelTypeColor(tunnel.type)}`}>
                        {getTunnelTypeLabel(tunnel.type)}
                      </span>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                      {tunnel.domain || '-'}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      <span className={`px-2 inline-flex text-xs leading-5 font-semibold rounded-full ${
                        tunnel.status.running
                          ? 'bg-green-100 text-green-800'
                          : 'bg-red-100 text-red-800'
                      }`}>
                        {tunnel.status.running ? '运行中' : '已停止'}
                      </span>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                      {tunnel.status.url ? (
                        <a
                          href={tunnel.status.url}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="text-blue-600 hover:text-blue-900"
                        >
                          {tunnel.status.url}
                        </a>
                      ) : '-'}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm font-medium space-x-2">
                      {tunnel.status.running ? (
                        <button
                          onClick={() => handleTunnelAction(tunnel.id, 'stop')}
                          className="text-yellow-600 hover:text-yellow-900"
                        >
                          停止
                        </button>
                      ) : (
                        <button
                          onClick={() => handleTunnelAction(tunnel.id, 'start')}
                          className="text-green-600 hover:text-green-900"
                        >
                          启动
                        </button>
                      )}
                      <button
                        onClick={() => handleTunnelAction(tunnel.id, 'delete')}
                        className="text-red-600 hover:text-red-900"
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