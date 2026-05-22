import { useState, useEffect } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { api } from '@/lib/api'

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
  const [certStatuses, setCertStatuses] = useState<Record<string, CertStatus>>(
    {}
  )
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
      const response = await api.get('/api/admin/certificate/list')
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
      const response = await api.post('/api/admin/certificate/generate', {
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
      await api.delete('/api/admin/certificate/delete', {
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
      const response = await api.post('/api/admin/certificate/renew', {
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
      const response = await api.post('/api/admin/certificate/auto-renew', {
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
      <div className='flex h-screen items-center justify-center'>
        <div className='text-lg'>加载中...</div>
      </div>
    )
  }

  return (
    <div className='container mx-auto p-6'>
      <div className='mb-6'>
        <h1 className='text-3xl font-bold'>证书管理</h1>
        <p className='mt-2 text-gray-600'>管理 SSL/TLS 证书</p>
      </div>

      {error && (
        <div className='mb-4 rounded border border-red-400 bg-red-100 px-4 py-3 text-red-700'>
          {error}
        </div>
      )}

      {/* 证书生成 */}
      <div className='mb-6 rounded-lg bg-white p-6 shadow'>
        <h2 className='mb-4 text-xl font-semibold'>生成新证书</h2>

        <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
          <div>
            <label className='mb-2 block text-sm font-medium text-gray-700'>
              域名 *
            </label>
            <input
              type='text'
              value={domain}
              onChange={(e) => setDomain(e.target.value)}
              placeholder='example.com'
              className='w-full rounded-md border border-gray-300 px-3 py-2'
            />
          </div>

          <div>
            <label className='mb-2 block text-sm font-medium text-gray-700'>
              证书类型
            </label>
            <select
              value={certType}
              onChange={(e) => setCertType(e.target.value)}
              className='w-full rounded-md border border-gray-300 px-3 py-2'
            >
              <option value='selfsigned'>自签名证书</option>
              <option value='acme'>Let's Encrypt (ACME)</option>
            </select>
          </div>

          {certType === 'acme' && (
            <div>
              <label className='mb-2 block text-sm font-medium text-gray-700'>
                邮箱地址 *
              </label>
              <input
                type='email'
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder='admin@example.com'
                className='w-full rounded-md border border-gray-300 px-3 py-2'
              />
            </div>
          )}

          <div>
            <label className='mb-2 block text-sm font-medium text-gray-700'>
              有效期 (天)
            </label>
            <input
              type='number'
              value={validityDays}
              onChange={(e) => setValidityDays(parseInt(e.target.value))}
              min={1}
              max={3650}
              className='w-full rounded-md border border-gray-300 px-3 py-2'
            />
          </div>
        </div>

        <div className='mt-4'>
          <button
            onClick={handleGenerateCert}
            disabled={generating || !domain}
            className='rounded bg-green-500 px-6 py-2 text-white hover:bg-green-600 disabled:opacity-50'
          >
            {generating ? '生成中...' : '生成证书'}
          </button>
        </div>
      </div>

      {/* 批量操作 */}
      <div className='mb-6 rounded-lg bg-white p-6 shadow'>
        <h2 className='mb-4 text-xl font-semibold'>批量操作</h2>

        <div className='flex space-x-4'>
          <button
            onClick={handleAutoRenew}
            className='rounded bg-blue-500 px-4 py-2 text-white hover:bg-blue-600'
          >
            自动更新所有证书
          </button>
        </div>
      </div>

      {/* 证书列表 */}
      <div className='rounded-lg bg-white p-6 shadow'>
        <h2 className='mb-4 text-xl font-semibold'>
          已安装的证书 ({certs.length})
        </h2>

        {certs.length === 0 ? (
          <div className='py-8 text-center text-gray-500'>暂无证书</div>
        ) : (
          <div className='overflow-x-auto'>
            <table className='min-w-full divide-y divide-gray-200'>
              <thead className='bg-gray-50'>
                <tr>
                  <th className='px-6 py-3 text-left text-xs font-medium tracking-wider text-gray-500 uppercase'>
                    域名
                  </th>
                  <th className='px-6 py-3 text-left text-xs font-medium tracking-wider text-gray-500 uppercase'>
                    类型
                  </th>
                  <th className='px-6 py-3 text-left text-xs font-medium tracking-wider text-gray-500 uppercase'>
                    状态
                  </th>
                  <th className='px-6 py-3 text-left text-xs font-medium tracking-wider text-gray-500 uppercase'>
                    过期时间
                  </th>
                  <th className='px-6 py-3 text-left text-xs font-medium tracking-wider text-gray-500 uppercase'>
                    自动更新
                  </th>
                  <th className='px-6 py-3 text-left text-xs font-medium tracking-wider text-gray-500 uppercase'>
                    操作
                  </th>
                </tr>
              </thead>
              <tbody className='divide-y divide-gray-200 bg-white'>
                {certs.map((cert) => {
                  const expiryStatus = getExpiryStatus(cert.domain)
                  return (
                    <tr key={cert.id}>
                      <td className='px-6 py-4 whitespace-nowrap'>
                        {cert.domain}
                      </td>
                      <td className='px-6 py-4 whitespace-nowrap'>
                        <span className='inline-flex rounded-full bg-blue-100 px-2 text-xs leading-5 font-semibold text-blue-800'>
                          {cert.cert_type}
                        </span>
                      </td>
                      <td className='px-6 py-4 whitespace-nowrap'>
                        <span
                          className={`inline-flex rounded-full px-2 text-xs leading-5 font-semibold ${
                            expiryStatus.color === 'green'
                              ? 'bg-green-100 text-green-800'
                              : expiryStatus.color === 'yellow'
                                ? 'bg-yellow-100 text-yellow-800'
                                : 'bg-red-100 text-red-800'
                          }`}
                        >
                          {expiryStatus.text}
                        </span>
                      </td>
                      <td className='px-6 py-4 text-sm whitespace-nowrap text-gray-500'>
                        {cert.expires_at
                          ? new Date(cert.expires_at).toLocaleDateString()
                          : '-'}
                      </td>
                      <td className='px-6 py-4 whitespace-nowrap'>
                        <input
                          type='checkbox'
                          checked={cert.auto_renew}
                          disabled
                          className='h-4 w-4 rounded text-blue-600'
                          readOnly
                        />
                      </td>
                      <td className='space-x-2 px-6 py-4 text-sm font-medium whitespace-nowrap'>
                        <button
                          onClick={() => handleRenewCert(cert.domain)}
                          className='text-blue-600 hover:text-blue-900'
                        >
                          更新
                        </button>
                        <button
                          onClick={() => handleDeleteCert(cert.domain)}
                          className='text-red-600 hover:text-red-900'
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
