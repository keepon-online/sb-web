import { createFileRoute } from '@tanstack/react-router'
import { useState, useEffect } from 'react'
import axios from 'axios'

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

export const Route = createFileRoute('/singbox/config')({
  component: SingboxConfigPage,
})

function SingboxConfigPage() {
  const [configs, setConfigs] = useState<Config[]>([])
  const [portStatus, setPortStatus] = useState<PortStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [generating, setGenerating] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // 配置生成表单状态
  const [selectedProtocol, setSelectedProtocol] = useState('vless')
  const [serverAddress, setServerAddress] = useState('')
  const [serverPort, setServerPort] = useState(443)
  const [domain, setDomain] = useState('')
  const [autoPort, setAutoPort] = useState(true)

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

  const handleGenerateConfig = async () => {
    if (!serverAddress) {
      setError('请输入服务器地址')
      return
    }

    setGenerating(true)
    setError(null)

    try {
      const options = {
        server: serverAddress,
        server_port: autoPort ? 0 : serverPort,
        domain: domain || serverAddress,
        path: '/',
        host: domain || serverAddress,
      }

      const response = await axios.post('/api/admin/singbox/config/generate', {
        protocol: selectedProtocol,
        options: options,
      })

      if (response.data.success) {
        // 自动保存配置
        const configName = `${selectedProtocol}-${serverAddress}-${response.data.port}`
        await saveConfig(response.data.config, configName)

        // 重新加载配置列表
        await loadConfigs()
        await loadPortStatus()

        // 重置表单
        setServerAddress('')
        setDomain('')
        setServerPort(443)
      }
    } catch (err) {
      setError('生成配置失败')
      console.error(err)
    } finally {
      setGenerating(false)
    }
  }

  const saveConfig = async (config: any, name: string) => {
    try {
      await axios.post('/api/admin/singbox/config/save', {
        name: name,
        config: config,
      })
    } catch (err) {
      console.error('保存配置失败:', err)
      throw err
    }
  }

  const handleDeleteConfig = async (id: number, name: string) => {
    if (!confirm(`确定要删除配置 "${name}" 吗？`)) return

    try {
      await axios.delete(`/api/admin/singbox/config/${id}`)
      await loadConfigs()
      await loadPortStatus()
    } catch (err) {
      setError('删除配置失败')
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
      setError('分配端口失败')
      console.error(err)
    }
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center h-screen">
        <div className="text-lg">加载中...</div>
      </div>
    )
  }

  return (
    <div className="container mx-auto p-6">
      <div className="mb-6">
        <h1 className="text-3xl font-bold">Sing-box 配置管理</h1>
        <p className="text-gray-600 mt-2">生成和管理 Sing-box 配置文件</p>
      </div>

      {error && (
        <div className="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded mb-4">
          {error}
        </div>
      )}

      {/* 配置生成表单 */}
      <div className="bg-white rounded-lg shadow p-6 mb-6">
        <h2 className="text-xl font-semibold mb-4">生成新配置</h2>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {/* 协议选择 */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-2">
              协议类型
            </label>
            <select
              value={selectedProtocol}
              onChange={(e) => setSelectedProtocol(e.target.value)}
              className="w-full border border-gray-300 rounded-md px-3 py-2"
            >
              <option value="vless">Vless (Reality)</option>
              <option value="vmess">Vmess (WebSocket)</option>
              <option value="hysteria2">Hysteria2</option>
              <option value="tuic">Tuic</option>
              <option value="anytls">Anytls</option>
            </select>
          </div>

          {/* 服务器地址 */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-2">
              服务器地址
            </label>
            <input
              type="text"
              value={serverAddress}
              onChange={(e) => setServerAddress(e.target.value)}
              placeholder="example.com 或 IP 地址"
              className="w-full border border-gray-300 rounded-md px-3 py-2"
            />
          </div>

          {/* 端口配置 */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-2">
              端口配置
            </label>
            <div className="flex items-center space-x-2">
              <input
                type="number"
                value={serverPort}
                onChange={(e) => setServerPort(parseInt(e.target.value))}
                disabled={autoPort}
                className="flex-1 border border-gray-300 rounded-md px-3 py-2 disabled:bg-gray-100"
                placeholder="端口"
              />
              <button
                onClick={handleAllocatePort}
                className="bg-blue-500 text-white px-3 py-2 rounded hover:bg-blue-600"
                title="随机分配端口"
              >
                🎲
              </button>
              <label className="flex items-center">
                <input
                  type="checkbox"
                  checked={autoPort}
                  onChange={(e) => setAutoPort(e.target.checked)}
                  className="mr-2"
                />
                自动
              </label>
            </div>
          </div>

          {/* 域名 */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-2">
              域名 (可选)
            </label>
            <input
              type="text"
              value={domain}
              onChange={(e) => setDomain(e.target.value)}
              placeholder="example.com"
              className="w-full border border-gray-300 rounded-md px-3 py-2"
            />
          </div>
        </div>

        {/* 协议说明 */}
        <div className="mt-4 p-4 bg-gray-50 rounded-md">
          <h3 className="font-medium mb-2">协议说明</h3>
          <div className="text-sm text-gray-600 space-y-1">
            <p><strong>Vless:</strong> 最新的轻量级协议，支持 Reality 加密</p>
            <p><strong>Vmess:</strong> 经典协议，支持 WebSocket 传输</p>
            <p><strong>Hysteria2:</strong> 高性能协议，基于 QUIC</p>
            <p><strong>Tuic:</strong> 基于 QUIC 的新协议</p>
            <p><strong>Anytls:</strong> 多协议兼容的传输层</p>
          </div>
        </div>

        {/* 生成按钮 */}
        <div className="mt-6">
          <button
            onClick={handleGenerateConfig}
            disabled={generating || !serverAddress}
            className="bg-green-500 text-white px-6 py-2 rounded hover:bg-green-600 disabled:opacity-50"
          >
            {generating ? '生成中...' : '生成配置'}
          </button>
        </div>
      </div>

      {/* 端口状态 */}
      {portStatus && (
        <div className="bg-white rounded-lg shadow p-6 mb-6">
          <h2 className="text-xl font-semibold mb-4">端口状态</h2>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            <div className="p-4 bg-blue-50 rounded-md">
              <div className="text-sm text-gray-600">已用端口</div>
              <div className="text-2xl font-bold text-blue-600">
                {portStatus.used_ports.length}
              </div>
            </div>
            <div className="p-4 bg-green-50 rounded-md">
              <div className="text-sm text-gray-600">可用端口</div>
              <div className="text-2xl font-bold text-green-600">
                {portStatus.available_count}
              </div>
            </div>
            <div className="p-4 bg-gray-50 rounded-md">
              <div className="text-sm text-gray-600">端口范围</div>
              <div className="text-2xl font-bold text-gray-600">
                {portStatus.range.min}-{portStatus.range.max}
              </div>
            </div>
          </div>
        </div>
      )}

      {/* 配置列表 */}
      <div className="bg-white rounded-lg shadow p-6">
        <h2 className="text-xl font-semibold mb-4">已保存的配置</h2>

        {configs.length === 0 ? (
          <div className="text-center py-8 text-gray-500">
            暂无保存的配置
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
                    协议
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    端口
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    状态
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    创建时间
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    操作
                  </th>
                </tr>
              </thead>
              <tbody className="bg-white divide-y divide-gray-200">
                {configs.map((config) => (
                  <tr key={config.id}>
                    <td className="px-6 py-4 whitespace-nowrap">{config.name}</td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      <span className="px-2 inline-flex text-xs leading-5 font-semibold rounded-full bg-blue-100 text-blue-800">
                        {config.protocol}
                      </span>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">{config.port}</td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      <span className={`px-2 inline-flex text-xs leading-5 font-semibold rounded-full ${
                        config.enabled
                          ? 'bg-green-100 text-green-800'
                          : 'bg-red-100 text-red-800'
                      }`}>
                        {config.enabled ? '启用' : '禁用'}
                      </span>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                      {new Date(config.created_at).toLocaleString()}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm font-medium">
                      <button
                        onClick={() => handleDeleteConfig(config.id, config.name)}
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
