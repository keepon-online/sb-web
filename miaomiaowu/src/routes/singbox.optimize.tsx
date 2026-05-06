import { createFileRoute } from '@tanstack/react-router'
import { useState, useEffect } from 'react'
import axios from 'axios'

interface BBRStatus {
  enabled: boolean
  version: string
  current_mode: string
  bbr_available: boolean
}

interface SystemResources {
  cpu: any
  memory: any
  disk: any
  network: any
}

export const Route = createFileRoute('/singbox/optimize')({
  component: SystemOptimizePage,
})

function SystemOptimizePage() {
  const [bbrStatus, setBbrStatus] = useState<BBRStatus | null>(null)
  const [systemResources, setSystemResources] = useState<SystemResources | null>(null)
  const [networkPerf, setNetworkPerf] = useState<any>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [successMessage, setSuccessMessage] = useState<string | null>(null)

  useEffect(() => {
    loadData()
    const interval = setInterval(loadData, 30000)
    return () => clearInterval(interval)
  }, [])

  const loadData = async () => {
    try {
      const [bbrRes, resourcesRes, networkRes] = await Promise.all([
        axios.get('/api/admin/system/bbr-status'),
        axios.get('/api/admin/system/resource-usage'),
        axios.get('/api/admin/system/network-performance'),
      ])

      setBbrStatus(bbrRes.data)
      setSystemResources(resourcesRes.data)
      setNetworkPerf(networkRes.data)
      setError(null)
    } catch (err) {
      console.error('加载系统数据失败:', err)
      setError('加载系统数据失败')
    } finally {
      setLoading(false)
    }
  }

  const handleOptimizeSystem = async (action: string, version?: string) => {
    setLoading(true)
    setError(null)

    try {
      const response = await axios.post('/api/admin/system/optimize', {
        action: action,
        version: version,
      })

      if (response.data.status === 'success') {
        setSuccessMessage('系统优化成功')
        await loadData()
      }
    } catch (err) {
      setError('系统优化失败')
      console.error(err)
    } finally {
      setLoading(false)
    }
  }

  const handleSpeedTest = async () => {
    setLoading(true)
    setError(null)

    try {
      const response = await axios.post('/api/admin/system/speed-test', {
        target: 'www.google.com',
      })

      setNetworkPerf(response.data)
      setSuccessMessage('网络测试完成')
    } catch (err) {
      setError('网络测试失败')
      console.error(err)
    } finally {
      setLoading(false)
    }
  }

  const generateSystemReport = async () => {
    setLoading(true)
    try {
      const response = await axios.get('/api/admin/system/report')
      const reportData = response.data

      // 创建并下载报告
      const blob = new Blob([JSON.stringify(reportData, null, 2)], {
        type: 'application/json',
      })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `system-report-${Date.now()}.json`
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      URL.revokeObjectURL(url)

      setSuccessMessage('系统报告已生成')
    } catch (err) {
      setError('生成报告失败')
      console.error(err)
    } finally {
      setLoading(false)
    }
  }

  if (loading && !bbrStatus) {
    return (
      <div className="flex items-center justify-center h-screen">
        <div className="text-lg">加载中...</div>
      </div>
    )
  }

  return (
    <div className="container mx-auto p-6">
      <div className="mb-6">
        <h1 className="text-3xl font-bold">系统优化</h1>
        <p className="text-gray-600 mt-2">BBR加速和系统性能优化</p>
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

      {/* BBR状态卡片 */}
      {bbrStatus && (
        <div className="bg-white rounded-lg shadow p-6 mb-6">
          <h2 className="text-xl font-semibold mb-4">BBR状态</h2>

          <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
            <div className="bg-gray-50 p-4 rounded">
              <div className="text-sm text-gray-600">BBR支持</div>
              <div className="text-lg font-semibold">
                {bbrStatus.bbr_available ? (
                  <span className="text-green-600">支持</span>
                ) : (
                  <span className="text-red-600">不支持</span>
                )}
              </div>
            </div>

            <div className="bg-gray-50 p-4 rounded">
              <div className="text-sm text-gray-600">BBR状态</div>
              <div className="text-lg font-semibold">
                {bbrStatus.enabled ? (
                  <span className="text-green-600">已启用</span>
                ) : (
                  <span className="text-gray-600">未启用</span>
                )}
              </div>
            </div>

            <div className="bg-gray-50 p-4 rounded">
              <div className="text-sm text-gray-600">当前版本</div>
              <div className="text-lg font-semibold">
                {bbrStatus.version || bbrStatus.current_mode || '-'}
              </div>
            </div>

            <div className="bg-gray-50 p-4 rounded">
              <div className="text-sm text-gray-600">拥塞控制</div>
              <div className="text-lg font-semibold">
                {bbrStatus.current_mode || '-'}
              </div>
            </div>
          </div>

          <div className="mt-4 flex space-x-4">
            {!bbrStatus.enabled && bbrStatus.bbr_available && (
              <>
                <button
                  onClick={() => handleOptimizeSystem('enable-bbr', 'bbr')}
                  disabled={loading}
                  className="bg-blue-500 text-white px-4 py-2 rounded hover:bg-blue-600 disabled:opacity-50"
                >
                  启用BBR
                </button>

                <button
                  onClick={() => handleOptimizeSystem('enable-bbr', 'bbr2')}
                  disabled={loading}
                  className="bg-purple-500 text-white px-4 py-2 rounded hover:bg-purple-600 disabled:opacity-50"
                >
                  启用BBR2
                </button>
              </>
            )}

            {bbrStatus.enabled && (
              <button
                onClick={() => handleOptimizeSystem('disable-bbr')}
                disabled={loading}
                className="bg-red-500 text-white px-4 py-2 rounded hover:bg-red-600 disabled:opacity-50"
              >
                禁用BBR
              </button>
            )}

            <button
              onClick={() => handleOptimizeSystem('optimize-all')}
              disabled={loading}
              className="bg-green-500 text-white px-4 py-2 rounded hover:bg-green-600 disabled:opacity-50"
            >
              全面优化
            </button>
          </div>
        </div>
      )}

      {/* 系统资源监控 */}
      {systemResources && (
        <div className="bg-white rounded-lg shadow p-6 mb-6">
          <h2 className="text-xl font-semibold mb-4">系统资源</h2>

          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
            {systemResources.cpu && (
              <div className="bg-blue-50 p-4 rounded">
                <div className="text-sm text-blue-600 mb-1">CPU使用率</div>
                <div className="text-2xl font-bold text-blue-800">
                  {systemResources.cpu.raw || 'N/A'}
                </div>
              </div>
            )}

            {systemResources.memory && (
              <div className="bg-green-50 p-4 rounded">
                <div className="text-sm text-green-600 mb-1">内存使用</div>
                <div className="text-2xl font-bold text-green-800">
                  {systemResources.memory.used || 'N/A'}
                </div>
                <div className="text-sm text-green-600">
                  总计: {systemResources.memory.total || 'N/A'}
                </div>
              </div>
            )}

            {systemResources.disk && (
              <div className="bg-yellow-50 p-4 rounded">
                <div className="text-sm text-yellow-600 mb-1">磁盘使用</div>
                <div className="text-2xl font-bold text-yellow-800">
                  {systemResources.disk.usage_percent || 'N/A'}
                </div>
                <div className="text-sm text-yellow-600">
                  已用: {systemResources.disk.used || 'N/A'}
                </div>
              </div>
            )}

            {systemResources.network && (
              <div className="bg-purple-50 p-4 rounded">
                <div className="text-sm text-purple-600 mb-1">网络接口</div>
                <div className="text-2xl font-bold text-purple-800">
                  {systemResources.network.interfaces?.length || 0}
                </div>
                <div className="text-sm text-purple-600">
                  个接口
                </div>
              </div>
            )}
          </div>
        </div>
      )}

      {/* 网络性能测试 */}
      <div className="bg-white rounded-lg shadow p-6 mb-6">
        <h2 className="text-xl font-semibold mb-4">网络性能测试</h2>

        <div className="flex space-x-4 mb-4">
          <button
            onClick={handleSpeedTest}
            disabled={loading}
            className="bg-blue-500 text-white px-4 py-2 rounded hover:bg-blue-600 disabled:opacity-50"
          >
            开始测试
          </button>

          <button
            onClick={generateSystemReport}
            disabled={loading}
            className="bg-green-500 text-white px-4 py-2 rounded hover:bg-green-600 disabled:opacity-50"
          >
            生成系统报告
          </button>
        </div>

        {networkPerf && networkPerf.results && (
          <div className="mt-4">
            <h3 className="text-lg font-medium mb-2">测试结果</h3>

            {networkPerf.results.ping_output && (
              <div className="bg-gray-50 p-3 rounded mb-2">
                <div className="text-sm font-medium text-gray-700">Ping测试</div>
                <pre className="text-xs text-gray-600 mt-1 whitespace-pre-wrap">
                  {networkPerf.results.ping_output}
                </pre>
              </div>
            )}

            {networkPerf.results.dns_output && (
              <div className="bg-gray-50 p-3 rounded">
                <div className="text-sm font-medium text-gray-700">DNS解析</div>
                <pre className="text-xs text-gray-600 mt-1 whitespace-pre-wrap">
                  {networkPerf.results.dns_output}
                </pre>
              </div>
            )}

            <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mt-4">
              {networkPerf.results.ping_duration_ms && (
                <div className="bg-blue-50 p-3 rounded">
                  <div className="text-xs text-blue-600">Ping延迟</div>
                  <div className="text-lg font-bold text-blue-800">
                    {networkPerf.results.ping_duration_ms}ms
                  </div>
                </div>
              )}

              {networkPerf.results.dns_duration_ms && (
                <div className="bg-green-50 p-3 rounded">
                  <div className="text-xs text-green-600">DNS延迟</div>
                  <div className="text-lg font-bold text-green-800">
                    {networkPerf.results.dns_duration_ms}ms
                  </div>
                </div>
              )}

              {networkPerf.interfaces && (
                <div className="bg-purple-50 p-3 rounded">
                  <div className="text-xs text-purple-600">网络接口</div>
                  <div className="text-lg font-bold text-purple-800">
                    {networkPerf.interfaces.length}
                  </div>
                </div>
              )}

              {networkPerf.active_connections !== undefined && (
                <div className="bg-yellow-50 p-3 rounded">
                  <div className="text-xs text-yellow-600">活动连接</div>
                  <div className="text-lg font-bold text-yellow-800">
                    {networkPerf.active_connections}
                  </div>
                </div>
              )}
            </div>
          </div>
        )}
      </div>

      {/* 网络接口详情 */}
      {networkPerf && networkPerf.interfaces && networkPerf.interfaces.length > 0 && (
        <div className="bg-white rounded-lg shadow p-6">
          <h2 className="text-xl font-semibold mb-4">网络接口</h2>

          <div className="overflow-x-auto">
            <table className="min-w-full divide-y divide-gray-200">
              <thead className="bg-gray-50">
                <tr>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    接口名称
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    状态
                  </th>
                </tr>
              </thead>
              <tbody className="bg-white divide-y divide-gray-200">
                {networkPerf.interfaces.map((iface: any, index: number) => (
                  <tr key={index}>
                    <td className="px-6 py-4 whitespace-nowrap">{iface.name}</td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      <span className={`px-2 inline-flex text-xs leading-5 font-semibold rounded-full ${
                        iface.state === 'up' ? 'bg-green-100 text-green-800' : 'bg-red-100 text-red-800'
                      }`}>
                        {iface.state}
                      </span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  )
}