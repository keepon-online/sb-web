import { createFileRoute } from '@tanstack/react-router'
import { useState, useEffect } from 'react'
import axios from 'axios'

interface Subscription {
  id: string
  name: string
  nodes: any[]
  format: string
  update_time: string
  enabled: boolean
  auto_update: boolean
  share_code: string
  user_code: string
  subscription_url: string
}

interface NodeInfo {
  name: string
  type: string
  link: string
}

export const Route = createFileRoute('/singbox/subscription')({
  component: SubscriptionPage,
})

function SubscriptionPage() {
  const [subscriptions, setSubscriptions] = useState<Subscription[]>([])
  const [selectedSubscription, setSelectedSubscription] = useState<Subscription | null>(null)
  const [exportContent, setExportContent] = useState<string>('')
  const [showExportModal, setShowExportModal] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [successMessage, setSuccessMessage] = useState<string | null>(null)

  // 表单状态
  const [showCreateForm, setShowCreateForm] = useState(false)
  const [formData, setFormData] = useState({
    name: '',
    format: 'clash',
  })

  // 节点链接状态
  const [nodeLinks, setNodeLinks] = useState<NodeInfo[]>([])
  const [showNodeLinks, setShowNodeLinks] = useState(false)

  useEffect(() => {
    loadSubscriptions()
  }, [])

  const loadSubscriptions = async () => {
    try {
      const response = await axios.get('/api/admin/subscription/list')
      setSubscriptions(response.data.subscriptions || [])
      setError(null)
    } catch (err) {
      console.error('加载订阅列表失败:', err)
      setError('加载订阅列表失败')
    } finally {
      setLoading(false)
    }
  }

  const handleCreateSubscription = async () => {
    if (!formData.name) {
      setError('请输入订阅名称')
      return
    }

    setLoading(true)
    setError(null)

    try {
      const response = await axios.post('/api/admin/subscription/generate', {
        name: formData.name,
        format: formData.format,
      })

      if (response.data.status === 'success') {
        setSuccessMessage('订阅创建成功')
        setShowCreateForm(false)
        setFormData({ name: '', format: 'clash' })
        await loadSubscriptions()
      }
    } catch (err) {
      setError('创建订阅失败')
      console.error(err)
    } finally {
      setLoading(false)
    }
  }

  const handleUpdateSubscription = async (subscriptionId: string) => {
    try {
      await axios.post('/api/admin/subscription/update', {
        subscription_id: subscriptionId,
      })

      setSuccessMessage('订阅更新成功')
      await loadSubscriptions()
    } catch (err) {
      setError('更新订阅失败')
      console.error(err)
    }
  }

  const handleDeleteSubscription = async (subscriptionId: string) => {
    if (!confirm('确定要删除此订阅吗？')) return

    try {
      await axios.delete('/api/admin/subscription/delete', {
        params: { subscription_id: subscriptionId },
      })

      setSuccessMessage('订阅删除成功')
      await loadSubscriptions()
    } catch (err) {
      setError('删除订阅失败')
      console.error(err)
    }
  }

  const handleExportSubscription = async (subscription: Subscription, format: string) => {
    try {
      const response = await axios.get('/api/admin/subscription/export', {
        params: {
          subscription_id: subscription.id,
          format: format,
        },
        responseType: format === 'json' ? 'json' : 'text',
      })

      let content = ''
      if (format === 'json') {
        content = JSON.stringify(response.data, null, 2)
      } else {
        content = response.data
      }

      setExportContent(content)
      setSelectedSubscription(subscription)
      setShowExportModal(true)
    } catch (err) {
      setError('导出订阅失败')
      console.error(err)
    }
  }

  const handleDownloadSubscription = () => {
    if (!selectedSubscription || !exportContent) return

    const format = selectedSubscription.format
    let mimeType = 'text/plain'
    let extension = 'txt'

    switch (format) {
      case 'clash':
        mimeType = 'text/yaml'
        extension = 'yaml'
        break
      case 'json':
        mimeType = 'application/json'
        extension = 'json'
        break
      case 'v2ray':
      case 'base64':
        mimeType = 'text/plain'
        extension = 'txt'
        break
    }

    const blob = new Blob([exportContent], { type: mimeType })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `${selectedSubscription.name}.${extension}`
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)

    setSuccessMessage('订阅已下载')
    setShowExportModal(false)
  }

  const handleShowNodeLinks = async (subscription: Subscription) => {
    try {
      const response = await axios.post('/api/admin/subscription/node-link', {
        subscription_id: subscription.id,
      })

      const links = response.data.links || []
      setNodeLinks(links)
      setSelectedSubscription(subscription)
      setShowNodeLinks(true)
    } catch (err) {
      setError('获取节点链接失败')
      console.error(err)
    }
  }

  const getFormatLabel = (format: string) => {
    const labels: Record<string, string> = {
      'clash': 'Clash',
      'v2ray': 'V2Ray',
      'singbox': 'Sing-box',
      'base64': 'Base64',
      'json': 'JSON',
    }
    return labels[format] || format
  }

  const getFormatColor = (format: string) => {
    const colors: Record<string, string> = {
      'clash': 'bg-blue-100 text-blue-800',
      'v2ray': 'bg-green-100 text-green-800',
      'singbox': 'bg-purple-100 text-purple-800',
      'base64': 'bg-yellow-100 text-yellow-800',
      'json': 'bg-gray-100 text-gray-800',
    }
    return colors[format] || 'bg-gray-100 text-gray-800'
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
        <h1 className="text-3xl font-bold">订阅管理</h1>
        <p className="text-gray-600 mt-2">生成和管理订阅链接</p>
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

      {/* 创建订阅表单 */}
      <div className="bg-white rounded-lg shadow p-6 mb-6">
        <h2 className="text-xl font-semibold mb-4">创建订阅</h2>

        {!showCreateForm ? (
          <button
            onClick={() => setShowCreateForm(true)}
            className="bg-blue-500 text-white px-4 py-2 rounded hover:bg-blue-600"
          >
            创建新订阅
          </button>
        ) : (
          <div>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">
                  订阅名称 *
                </label>
                <input
                  type="text"
                  value={formData.name}
                  onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                  placeholder="my-subscription"
                  className="w-full border border-gray-300 rounded-md px-3 py-2"
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">
                  订阅格式
                </label>
                <select
                  value={formData.format}
                  onChange={(e) => setFormData({ ...formData, format: e.target.value })}
                  className="w-full border border-gray-300 rounded-md px-3 py-2"
                >
                  <option value="clash">Clash</option>
                  <option value="v2ray">V2Ray</option>
                  <option value="singbox">Sing-box</option>
                  <option value="base64">Base64</option>
                  <option value="json">JSON</option>
                </select>
              </div>
            </div>

            <div className="mt-4 flex space-x-4">
              <button
                onClick={handleCreateSubscription}
                disabled={loading}
                className="bg-green-500 text-white px-6 py-2 rounded hover:bg-green-600 disabled:opacity-50"
              >
                {loading ? '创建中...' : '创建订阅'}
              </button>

              <button
                onClick={() => {
                  setShowCreateForm(false)
                  setFormData({ name: '', format: 'clash' })
                }}
                className="bg-gray-500 text-white px-6 py-2 rounded hover:bg-gray-600"
              >
                取消
              </button>
            </div>
          </div>
        )}
      </div>

      {/* 订阅列表 */}
      <div className="bg-white rounded-lg shadow p-6">
        <h2 className="text-xl font-semibold mb-4">
          订阅列表 ({subscriptions.length})
        </h2>

        {subscriptions.length === 0 ? (
          <div className="text-center py-8 text-gray-500">
            暂无订阅，点击上方按钮创建
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
                    格式
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    节点数量
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    订阅URL
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    更新时间
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    操作
                  </th>
                </tr>
              </thead>
              <tbody className="bg-white divide-y divide-gray-200">
                {subscriptions.map((subscription) => (
                  <tr key={subscription.id}>
                    <td className="px-6 py-4 whitespace-nowrap">{subscription.name}</td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      <span className={`px-2 inline-flex text-xs leading-5 font-semibold rounded-full ${getFormatColor(subscription.format)}`}>
                        {getFormatLabel(subscription.format)}
                      </span>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                      {subscription.nodes?.length || 0}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-blue-600">
                      {subscription.subscription_url ? (
                        <a
                          href={subscription.subscription_url}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="hover:text-blue-900"
                        >
                          {subscription.subscription_url.substring(0, 50)}...
                        </a>
                      ) : '-'}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                      {new Date(subscription.update_time).toLocaleString()}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm font-medium space-x-2">
                      <button
                        onClick={() => handleUpdateSubscription(subscription.id)}
                        className="text-blue-600 hover:text-blue-900"
                      >
                        更新
                      </button>
                      <button
                        onClick={() => handleExportSubscription(subscription, subscription.format)}
                        className="text-green-600 hover:text-green-900"
                      >
                        导出
                      </button>
                      <button
                        onClick={() => handleShowNodeLinks(subscription)}
                        className="text-purple-600 hover:text-purple-900"
                      >
                        节点链接
                      </button>
                      <button
                        onClick={() => handleDeleteSubscription(subscription.id)}
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

      {/* 导出模态框 */}
      {showExportModal && selectedSubscription && (
        <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
          <div className="bg-white rounded-lg p-6 max-w-4xl w-full mx-4">
            <h3 className="text-lg font-semibold mb-4">
              导出订阅 - {selectedSubscription.name}
            </h3>

            <div className="mb-4">
              <label className="block text-sm font-medium text-gray-700 mb-2">
                订阅内容
              </label>
              <textarea
                value={exportContent}
                readOnly
                rows={20}
                className="w-full border border-gray-300 rounded-md px-3 py-2 font-mono text-sm"
              />
            </div>

            <div className="flex justify-end space-x-4">
              <button
                onClick={() => setShowExportModal(false)}
                className="bg-gray-500 text-white px-4 py-2 rounded hover:bg-gray-600"
              >
                关闭
              </button>
              <button
                onClick={handleDownloadSubscription}
                className="bg-blue-500 text-white px-4 py-2 rounded hover:bg-blue-600"
              >
                下载
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 节点链接模态框 */}
      {showNodeLinks && selectedSubscription && (
        <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
          <div className="bg-white rounded-lg p-6 max-w-4xl w-full mx-4 max-h-screen overflow-y-auto">
            <h3 className="text-lg font-semibold mb-4">
              节点链接 - {selectedSubscription.name}
            </h3>

            <div className="space-y-2">
              {nodeLinks.map((node, index) => (
                <div key={index} className="bg-gray-50 p-3 rounded">
                  <div className="text-sm font-medium text-gray-700 mb-1">
                    {node.name} ({node.type})
                  </div>
                  <input
                    type="text"
                    value={node.link}
                    readOnly
                    className="w-full border border-gray-300 rounded px-3 py-2 text-xs font-mono"
                  />
                </div>
              ))}
            </div>

            <div className="mt-4 flex justify-end">
              <button
                onClick={() => setShowNodeLinks(false)}
                className="bg-gray-500 text-white px-4 py-2 rounded hover:bg-gray-600"
              >
                关闭
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
