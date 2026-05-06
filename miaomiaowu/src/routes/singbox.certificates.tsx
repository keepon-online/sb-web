import { createFileRoute } from '@tanstack/react-router'
import { useState, useEffect } from 'react'
import axios from 'axios'

interface Certificate {
  id: number
  domain: string
  cert_type: string
  cert_path: string
  key_path: string
  expires_at: string
  auto_renew: boolean
  acme_email: string
  fingerprint: string
}

interface CertStatus {
  domain: string
  valid: boolean
  expires_in_days: number
  expires_at: string
  fingerprint: string
}

export const Route = createFileRoute('/singbox/certificates')({
  component: CertificatesPage,
})

function CertificatesPage() {
  const [certs, setCerts] = useState<Certificate[]>([])
  const [certStatuses, setCertStatuses] = useState<Record<string, CertStatus>>({})
  const [loading, setLoading] = useState(true)
  const [generating, setGenerating] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // 表单状态
  const [domain, setDomain] = useState('')
  const [certType, setCertType] = useState('selfsigned')
  const [validityDays, setValidityDays] = useState(365)
  const [email, setEmail] = useState('')

  useEffect(() => {
    loadCertificates()
  }, [])

  const loadCertificates = async () => {
    try {
      const response = await axios.get('/api/admin/cert/list')
      setCerts(response.data.certs || [])
      setCertStatuses(response.data.cert_status || {})
    } catch (err) {
      console.error('加载证书失败:', err)
      setError('加载证书失败')
    } finally {
      setLoading(false)
    }
  }

  const handleGenerateCert = async () => {
    if (!domain) {
      setError('请输入域名')
      return
    }

    setGenerating(true)
    setError(null)

    try {
      const response = await axios.post('/api/admin/cert/generate', {
        domain: domain,
        cert_type: certType,
        validity_days: validityDays,
        email: email,
        auto_renew: true,
      })

      if (response.data.status === 'success') {
        await loadCertificates()
        setDomain('')
        setEmail('')
      }
    } catch (err) {
      setError('生成证书失败')
      console.error(err)
    } finally {
      setGenerating(false)
    }
  }

  const handleDeleteCert = async (domain: string) => {
    if (!confirm(`确定要删除证书 "${domain}" 吗？`)) return

    try {
      await axios.delete('/api/admin/cert/delete', {
        params: { domain: domain },
      })

      await loadCertificates()
    } catch (err) {
      setError('删除证书失败')
      console.error(err)
    }
  }

  const handleRenewCert = async (domain: string) => {
    try {
      const response = await axios.post('/api/admin/cert/renew', {
        domain: domain,
        warn_days: 30,
        force: false,
      })

      if (response.data.status === 'success') {
        await loadCertificates()
      }
    } catch (err) {
      setError('更新证书失败')
      console.error(err)
    }
  }

  const handleAutoRenew = async () => {
    try {
      const response = await axios.post('/api/admin/cert/auto-renew', {
        warn_days: 30,
      })

      if (response.data.status === 'success') {
        await loadCertificates()
      }
    } catch (err) {
      setError('自动更新失败')
      console.error(err)
    }
  }

  const getExpiryStatus = (domain: string) => {
    const status = certStatuses[domain]
    if (!status) return { text: '未知', color: 'gray' }

    if (!status.valid) {
      return { text: '已过期', color: 'red' }
    }

    if (status.expires_in_days <= 7) {
      return { text: `${status.expires_in_days}天后过期`, color: 'red' }
    } else if (status.expires_in_days <= 30) {
      return { text: `${status.expires_in_days}天后过期`, color: 'yellow' }
    } else {
      return { text: `${status.expires_in_days}天后过期`, color: 'green' }
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
        <h1 className="text-3xl font-bold">证书管理</h1>
        <p className="text-gray-600 mt-2">管理 SSL/TLS 证书</p>
      </div>

      {error && (
        <div className="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded mb-4">
          {error}
        </div>
      )}

      {/* 证书生成 */}
      <div className="bg-white rounded-lg shadow p-6 mb-6">
        <h2 className="text-xl font-semibold mb-4">生成新证书</h2>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-2">
              域名 *
            </label>
            <input
              type="text"
              value={domain}
              onChange={(e) => setDomain(e.target.value)}
              placeholder="example.com"
              className="w-full border border-gray-300 rounded-md px-3 py-2"
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700 mb-2">
              证书类型
            </label>
            <select
              value={certType}
              onChange={(e) => setCertType(e.target.value)}
              className="w-full border border-gray-300 rounded-md px-3 py-2"
            >
              <option value="selfsigned">自签名证书</option>
              <option value="acme">Let's Encrypt (ACME)</option>
            </select>
          </div>

          {certType === 'acme' && (
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">
                邮箱地址 *
              </label>
              <input
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="admin@example.com"
                className="w-full border border-gray-300 rounded-md px-3 py-2"
              />
            </div>
          )}

          <div>
            <label className="block text-sm font-medium text-gray-700 mb-2">
              有效期 (天)
            </label>
            <input
              type="number"
              value={validityDays}
              onChange={(e) => setValidityDays(parseInt(e.target.value))}
              min={1}
              max={3650}
              className="w-full border border-gray-300 rounded-md px-3 py-2"
            />
          </div>
        </div>

        <div className="mt-4">
          <button
            onClick={handleGenerateCert}
            disabled={generating || !domain}
            className="bg-green-500 text-white px-6 py-2 rounded hover:bg-green-600 disabled:opacity-50"
          >
            {generating ? '生成中...' : '生成证书'}
          </button>
        </div>
      </div>

      {/* 批量操作 */}
      <div className="bg-white rounded-lg shadow p-6 mb-6">
        <h2 className="text-xl font-semibold mb-4">批量操作</h2>

        <div className="flex space-x-4">
          <button
            onClick={handleAutoRenew}
            className="bg-blue-500 text-white px-4 py-2 rounded hover:bg-blue-600"
          >
            自动更新所有证书
          </button>
        </div>
      </div>

      {/* 证书列表 */}
      <div className="bg-white rounded-lg shadow p-6">
        <h2 className="text-xl font-semibold mb-4">已安装的证书 ({certs.length})</h2>

        {certs.length === 0 ? (
          <div className="text-center py-8 text-gray-500">
            暂无证书
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="min-w-full divide-y divide-gray-200">
              <thead className="bg-gray-50">
                <tr>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    域名
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    类型
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    状态
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    过期时间
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    自动更新
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    操作
                  </th>
                </tr>
              </thead>
              <tbody className="bg-white divide-y divide-gray-200">
                {certs.map((cert) => {
                  const expiryStatus = getExpiryStatus(cert.domain)
                  return (
                    <tr key={cert.id}>
                      <td className="px-6 py-4 whitespace-nowrap">{cert.domain}</td>
                      <td className="px-6 py-4 whitespace-nowrap">
                        <span className="px-2 inline-flex text-xs leading-5 font-semibold rounded-full bg-blue-100 text-blue-800">
                          {cert.cert_type}
                        </span>
                      </td>
                      <td className="px-6 py-4 whitespace-nowrap">
                        <span className={`px-2 inline-flex text-xs leading-5 font-semibold rounded-full ${
                          expiryStatus.color === 'green'
                            ? 'bg-green-100 text-green-800'
                            : expiryStatus.color === 'yellow'
                            ? 'bg-yellow-100 text-yellow-800'
                            : 'bg-red-100 text-red-800'
                        }`}>
                          {expiryStatus.text}
                        </span>
                      </td>
                      <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                        {cert.expires_at ? new Date(cert.expires_at).toLocaleDateString() : '-'}
                      </td>
                      <td className="px-6 py-4 whitespace-nowrap">
                        <input
                          type="checkbox"
                          checked={cert.auto_renew}
                          disabled
                          className="w-4 h-4 text-blue-600 rounded"
                          readOnly
                        />
                      </td>
                      <td className="px-6 py-4 whitespace-nowrap text-sm font-medium space-x-2">
                        <button
                          onClick={() => handleRenewCert(cert.domain)}
                          className="text-blue-600 hover:text-blue-900"
                        >
                          更新
                        </button>
                        <button
                          onClick={() => handleDeleteCert(cert.domain)}
                          className="text-red-600 hover:text-red-900"
                        >
                          删除
                        </button>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}
