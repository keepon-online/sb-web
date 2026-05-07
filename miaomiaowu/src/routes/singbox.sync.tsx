import { createFileRoute } from '@tanstack/react-router'
import { useState, useEffect } from 'react'
import axios from 'axios'

interface ShareConfig {
  id: string
  name: string
  target: string
  enabled: boolean
  auto_share: boolean
  last_shared: string
  url: string
  created_at: string
}

interface Subscription {
  id: string
  name: string
}

export const Route = createFileRoute('/singbox/sync')({
  component: GitLabSyncPage,
})

function GitLabSyncPage() {
  const [shareConfigs, setShareConfigs] = useState<ShareConfig[]>([])
  const [subscriptions, setSubscriptions] = useState<Subscription[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [successMessage, setSuccessMessage] = useState<string | null>(null)

  // 表单状态
  const [showSyncForm, setShowSyncForm] = useState(false)
  const [syncTarget, setSyncTarget] = useState('gitlab')
  const [syncFormData, setSyncFormData] = useState({
    subscription_id: '',
    token: '',
    repo_url: '',
    file_path: '',
    branch: 'main',
    message: '',
  })

  useEffect(() => {
    loadData()
  }, [])

  const loadData = async () => {
    try {
      const [sharesRes, subsRes] = await Promise.all([
        axios.get('/api/admin/share/list'),
        axios.get('/api/admin/subscription/list'),
      ])

      setShareConfigs(sharesRes.data.share_configs || [])
      setSubscriptions(subsRes.data.subscriptions || [])
      setError(null)
    } catch (err) {
      console.error('加载数据失败:', err)
      setError('加载数据失败')
    } finally {
      setLoading(false)
    }
  }

  const handleSync = async () => {
    if (!syncFormData.subscription_id) {
      setError('请选择订阅')
      return
    }
    if (!syncFormData.token) {
      setError('请输入访问令牌')
      return
    }
    if (!syncFormData.repo_url) {
      setError('请输入仓库URL')
      return
    }

    setLoading(true)
    setError(null)

    try {
      let endpoint = ''
      switch (syncTarget) {
        case 'gitlab':
          endpoint = '/api/admin/gitlab/sync'
          break
        case 'github':
          endpoint = '/api/admin/github/sync'
          break
        case 'pastebin':
          endpoint = '/api/admin/pastebin/share'
          break
        default:
          endpoint = '/api/admin/gitlab/sync'
      }

      const response = await axios.post(endpoint, {
        subscription_id: syncFormData.subscription_id,
        token: syncFormData.token,
        repo_url: syncFormData.repo_url,
        file_path: syncFormData.file_path || 'subscriptions/config.json',
        branch: syncFormData.branch,
        message: syncFormData.message,
      })

      if (response.data.status === 'success') {
        setSuccessMessage('同步成功')
        setShowSyncForm(false)
        setSyncFormData({
          subscription_id: '',
          token: '',
          repo_url: '',
          file_path: '',
          branch: 'main',
          message: '',
        })
        await loadData()
      }
    } catch (err) {
      setError('同步失败')
      console.error(err)
    } finally {
      setLoading(false)
    }
  }

  const handleDeleteShare = async (shareId: string) => {
    if (!confirm('确定要删除此分享配置吗？')) return

    try {
      await axios.delete('/api/admin/share/delete', {
        params: { share_id: shareId },
      })

      setSuccessMessage('分享配置删除成功')
      await loadData()
    } catch (err) {
      setError('删除分享配置失败')
      console.error(err)
    }
  }

  const getTargetLabel = (target: string) => {
    const labels: Record<string, string> = {
      'gitlab': 'GitLab',
      'github': 'GitHub',
      'gist': 'Gist',
      'pastebin': 'Pastebin',
      'local': '本地',
    }
    return labels[target] || target
  }

  const getTargetColor = (target: string) => {
    const colors: Record<string, string> = {
      'gitlab': 'bg-orange-100 text-orange-800',
      'github': 'bg-gray-800 text-white',
      'gist': 'bg-purple-100 text-purple-800',
      'pastebin': 'bg-green-100 text-green-800',
      'local': 'bg-blue-100 text-blue-800',
    }
    return colors[target] || 'bg-gray-100 text-gray-800'
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
        <h1 className="text-3xl font-bold">Git同步</h1>
        <p className="text-gray-600 mt-2">将订阅同步到GitLab、GitHub等平台</p>
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

      {/* 同步表单 */}
      <div className="bg-white rounded-lg shadow p-6 mb-6">
        <h2 className="text-xl font-semibold mb-4">同步到平台</h2>

        {!showSyncForm ? (
          <div className="flex space-x-4">
            <button
              onClick={() => {
                setShowSyncForm(true)
                setSyncTarget('gitlab')
              }}
              className="bg-orange-500 text-white px-4 py-2 rounded hover:bg-orange-600"
            >
              GitLab同步
            </button>

            <button
              onClick={() => {
                setShowSyncForm(true)
                setSyncTarget('github')
              }}
              className="bg-gray-800 text-white px-4 py-2 rounded hover:bg-gray-900"
            >
              GitHub同步
            </button>

            <button
              onClick={() => {
                setShowSyncForm(true)
                setSyncTarget('pastebin')
              }}
              className="bg-green-500 text-white px-4 py-2 rounded hover:bg-green-600"
            >
              Pastebin分享
            </button>
          </div>
        ) : (
          <div>
            <h3 className="text-lg font-medium mb-4">
              同步到 {getTargetLabel(syncTarget)}
            </h3>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">
                  选择订阅 *
                </label>
                <select
                  value={syncFormData.subscription_id}
                  onChange={(e) => setSyncFormData({ ...syncFormData, subscription_id: e.target.value })}
                  className="w-full border border-gray-300 rounded-md px-3 py-2"
                >
                  <option value="">请选择订阅</option>
                  {subscriptions.map((sub) => (
                    <option key={sub.id} value={sub.id}>
                      {sub.name}
                    </option>
                  ))}
                </select>
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">
                  访问令牌 *
                </label>
                <input
                  type="password"
                  value={syncFormData.token}
                  onChange={(e) => setSyncFormData({ ...syncFormData, token: e.target.value })}
                  placeholder="Personal Access Token"
                  className="w-full border border-gray-300 rounded-md px-3 py-2"
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">
                  仓库URL *
                </label>
                <input
                  type="text"
                  value={syncFormData.repo_url}
                  onChange={(e) => setSyncFormData({ ...syncFormData, repo_url: e.target.value })}
                  placeholder="https://github.com/username/repo"
                  className="w-full border border-gray-300 rounded-md px-3 py-2"
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">
                  文件路径
                </label>
                <input
                  type="text"
                  value={syncFormData.file_path}
                  onChange={(e) => setSyncFormData({ ...syncFormData, file_path: e.target.value })}
                  placeholder="subscriptions/config.json"
                  className="w-full border border-gray-300 rounded-md px-3 py-2"
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">
                  分支名称
                </label>
                <input
                  type="text"
                  value={syncFormData.branch}
                  onChange={(e) => setSyncFormData({ ...syncFormData, branch: e.target.value })}
                  placeholder="main"
                  className="w-full border border-gray-300 rounded-md px-3 py-2"
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">
                  提交消息
                </label>
                <input
                  type="text"
                  value={syncFormData.message}
                  onChange={(e) => setSyncFormData({ ...syncFormData, message: e.target.value })}
                  placeholder="Update subscription"
                  className="w-full border border-gray-300 rounded-md px-3 py-2"
                />
              </div>
            </div>

            <div className="mt-4 flex space-x-4">
              <button
                onClick={handleSync}
                disabled={loading}
                className="bg-blue-500 text-white px-6 py-2 rounded hover:bg-blue-600 disabled:opacity-50"
              >
                {loading ? '同步中...' : '开始同步'}
              </button>

              <button
                onClick={() => {
                  setShowSyncForm(false)
                  setSyncFormData({
                    subscription_id: '',
                    token: '',
                    repo_url: '',
                    file_path: '',
                    branch: 'main',
                    message: '',
                  })
                }}
                className="bg-gray-500 text-white px-6 py-2 rounded hover:bg-gray-600"
              >
                取消
              </button>
            </div>
          </div>
        )}
      </div>

      {/* 分享配置列表 */}
      <div className="bg-white rounded-lg shadow p-6">
        <h2 className="text-xl font-semibold mb-4">同步配置列表</h2>

        {shareConfigs.length === 0 ? (
          <div className="text-center py-8 text-gray-500">
            暂无同步配置
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
                    目标平台
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    状态
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    分享URL
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    最后同步
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    操作
                  </th>
                </tr>
              </thead>
              <tbody className="bg-white divide-y divide-gray-200">
                {shareConfigs.map((config) => (
                  <tr key={config.id}>
                    <td className="px-6 py-4 whitespace-nowrap">{config.name}</td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      <span className={`px-2 inline-flex text-xs leading-5 font-semibold rounded-full ${getTargetColor(config.target)}`}>
                        {getTargetLabel(config.target)}
                      </span>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      <span className={`px-2 inline-flex text-xs leading-5 font-semibold rounded-full ${
                        config.enabled
                          ? 'bg-green-100 text-green-800'
                          : 'bg-gray-100 text-gray-800'
                      }`}>
                        {config.enabled ? '已启用' : '未启用'}
                      </span>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-blue-600">
                      {config.url ? (
                        <a
                          href={config.url}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="hover:text-blue-900"
                        >
                          {config.url.substring(0, 50)}...
                        </a>
                      ) : '-'}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                      {config.last_shared
                        ? new Date(config.last_shared).toLocaleString()
                        : '未同步'}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm font-medium">
                      <button
                        onClick={() => handleDeleteShare(config.id)}
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

      {/* 使用说明 */}
      <div className="bg-white rounded-lg shadow p-6 mt-6">
        <h2 className="text-xl font-semibold mb-4">使用说明</h2>

        <div className="space-y-4">
          <div>
            <h3 className="text-lg font-medium text-gray-800">GitLab配置</h3>
            <ul className="list-disc list-inside text-sm text-gray-600 mt-2 space-y-1">
              <li>创建Personal Access Token: 用户设置 → 访问令牌 → api, write_repository</li>
              <li>仓库URL格式: https://gitlab.com/username/repo</li>
              <li>文件路径: 相对于仓库根目录的路径</li>
            </ul>
          </div>

          <div>
            <h3 className="text-lg font-medium text-gray-800">GitHub配置</h3>
            <ul className="list-disc list-inside text-sm text-gray-600 mt-2 space-y-1">
              <li>创建Personal Access Token: Settings → Developer settings → Personal access tokens</li>
              <li>仓库URL格式: https://github.com/username/repo</li>
              <li>需要repo权限</li>
            </ul>
          </div>

          <div>
            <h3 className="text-lg font-medium text-gray-800">Pastebin配置</h3>
            <ul className="list-disc list-inside text-sm text-gray-600 mt-2 space-y-1">
              <li>获取Developer API Key: https://pastebin.com/doc_api</li>
              <li>内容将自动设置为1天过期</li>
            </ul>
          </div>
        </div>
      </div>
    </div>
  )
}
