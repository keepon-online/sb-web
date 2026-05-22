// @ts-nocheck
import { useState, useEffect, useCallback, useRef } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { createFileRoute, redirect } from '@tanstack/react-router'
import { Plus, Pencil, Trash2, Eye, Upload, Save, X } from 'lucide-react'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'
import { api } from '@/lib/api'
import {
  extractProxyGroups,
  extractTemplateVariables,
  updateProxyGroups,
  createDefaultFormState,
  parseTemplate,
  generateProxyGroupsPreview,
  generateRegionProxyGroups,
  getRegionProxyGroupNames,
  PROXY_NODES_MARKER,
  PROXY_PROVIDERS_MARKER,
  REGION_PROXY_GROUPS_MARKER,
  PROXY_NODES_DISPLAY,
  PROXY_PROVIDERS_DISPLAY,
  REGION_PROXY_GROUPS_DISPLAY,
  type ProxyGroupFormState,
} from '@/lib/template-v3-utils'
import { cn } from '@/lib/utils'
import { useMediaQuery } from '@/hooks/use-media-query'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { DataTable } from '@/components/data-table'
import type { DataTableColumn } from '@/components/data-table'
import { ProxyGroupEditor } from '@/components/template-v3/proxy-group-editor'
import { TemplatePreview } from '@/components/template-v3/template-preview'
import { TemplateUploadDialog } from '@/components/template-v3/template-upload-dialog'

const TEMPLATE_DRAFT_KEY_PREFIX = 'mmw_template_v3_draft_'

export const Route = createFileRoute('/templates-v3/')({
  beforeLoad: () => {
    const token = useAuthStore.getState().auth.accessToken
    if (!token) {
      throw redirect({ to: '/' })
    }
  },
  component: TemplatesV3Page,
})

function TemplatesV3Page() {
  const queryClient = useQueryClient()
  const isMobile = useMediaQuery('(max-width: 767px)')
  const isTablet = useMediaQuery('(min-width: 768px) and (max-width: 1024px)')
  const isDesktop = useMediaQuery('(min-width: 1025px)')

  // Dialog states
  const [isEditorOpen, setIsEditorOpen] = useState(false)
  const [isUploadDialogOpen, setIsUploadDialogOpen] = useState(false)
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false)
  const [isRenameDialogOpen, setIsRenameDialogOpen] = useState(false)
  const [isCloseConfirmOpen, setIsCloseConfirmOpen] = useState(false)
  const [isDraftRecoveryOpen, setIsDraftRecoveryOpen] = useState(false)

  // Editing state
  const [editingTemplateName, setEditingTemplateName] = useState<string | null>(
    null
  )
  const [templateContent, setTemplateContent] = useState('')
  const [proxyGroups, setProxyGroups] = useState<ProxyGroupFormState[]>([])
  const [editorTab, setEditorTab] = useState<'visual' | 'yaml'>('visual')
  const [isDirty, setIsDirty] = useState(false)
  const isInitLoadRef = useRef(false)
  const pendingDraftRef = useRef<any>(null)
  const [enableRegionProxyGroups, setEnableRegionProxyGroups] = useState(false)
  const [templateVariables, setTemplateVariables] = useState<
    Record<string, string>
  >({})

  // Delete/Rename state
  const [deletingTemplateName, setDeletingTemplateName] = useState<
    string | null
  >(null)
  const [renamingTemplate, setRenamingTemplate] = useState<string | null>(null)
  const [newTemplateName, setNewTemplateName] = useState('')

  // Preview state
  const [previewContent, setPreviewContent] = useState('')
  const [isPreviewLoading, setIsPreviewLoading] = useState(false)
  const [isPreviewOpen, setIsPreviewOpen] = useState(false)

  // List preview state (for eye button in table)
  const [listPreviewOpen, setListPreviewOpen] = useState(false)
  const [listPreviewContent, setListPreviewContent] = useState('')
  const [listPreviewLoading, setListPreviewLoading] = useState(false)
  const [listPreviewTemplateName, setListPreviewTemplateName] = useState<
    string | null
  >(null)
  const [listPreviewTemplateContent, setListPreviewTemplateContent] =
    useState('')

  // Fetch templates list
  const { data: templates = [], isLoading } = useQuery<string[]>({
    queryKey: ['rule-templates'],
    queryFn: async () => {
      const response = await api.get('/api/admin/rule-templates')
      return response.data.templates || []
    },
  })

  // Fetch template content when editing
  const { data: templateData } = useQuery({
    queryKey: ['rule-template', editingTemplateName],
    queryFn: async () => {
      const response = await api.get(
        `/api/admin/rule-templates/${encodeURIComponent(editingTemplateName!)}`
      )
      return response.data.content as string
    },
    enabled: !!editingTemplateName && isEditorOpen,
  })

  // Fetch nodes for preview
  const { data: nodesData } = useQuery({
    queryKey: ['nodes-for-preview'],
    queryFn: async () => {
      const response = await api.get('/api/admin/nodes')
      const nodes = response.data.nodes || []
      // Convert nodes to Clash format by parsing clash_config
      return nodes
        .map((node: any) => {
          if (node.clash_config) {
            try {
              return JSON.parse(node.clash_config)
            } catch {
              return { name: node.node_name, type: node.protocol }
            }
          }
          return { name: node.node_name, type: node.protocol }
        })
        .filter((n: any) => n.name && n.type)
    },
    enabled: isEditorOpen,
  })

  // Update template mutation
  const updateMutation = useMutation({
    mutationFn: async ({
      name,
      content,
    }: {
      name: string
      content: string
    }) => {
      await api.put(`/api/admin/rule-templates/${encodeURIComponent(name)}`, {
        content,
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['rule-templates'] })
      queryClient.invalidateQueries({
        queryKey: ['rule-template', editingTemplateName],
      })
      if (editingTemplateName) {
        localStorage.removeItem(TEMPLATE_DRAFT_KEY_PREFIX + editingTemplateName)
      }
      toast.success('模板保存成功')
      setIsDirty(false)
      // Close editor after successful save
      setIsEditorOpen(false)
      setEditingTemplateName(null)
      setTemplateContent('')
      setProxyGroups([])
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '保存失败')
    },
  })

  // Delete template mutation
  const deleteMutation = useMutation({
    mutationFn: async (name: string) => {
      await api.delete(`/api/admin/rule-templates/${encodeURIComponent(name)}`)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['rule-templates'] })
      toast.success('模板已删除')
      setIsDeleteDialogOpen(false)
      setDeletingTemplateName(null)
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '删除失败')
    },
  })

  // Upload template mutation
  const uploadMutation = useMutation({
    mutationFn: async (file: File) => {
      const formData = new FormData()
      formData.append('template', file)
      await api.post('/api/admin/rule-templates/upload', formData, {
        headers: { 'Content-Type': 'multipart/form-data' },
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['rule-templates'] })
      toast.success('模板上传成功')
      setIsUploadDialogOpen(false)
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '上传失败')
    },
  })

  // Create template mutation (for paste/blank)
  const createMutation = useMutation({
    mutationFn: async ({
      name,
      content,
    }: {
      name: string
      content: string
    }) => {
      const formData = new FormData()
      const blob = new Blob([content], { type: 'text/yaml' })
      formData.append('template', blob, name)
      await api.post('/api/admin/rule-templates/upload', formData, {
        headers: { 'Content-Type': 'multipart/form-data' },
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['rule-templates'] })
      toast.success('模板创建成功')
      setIsUploadDialogOpen(false)
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '创建失败')
    },
  })

  // Rename template mutation
  const renameMutation = useMutation({
    mutationFn: async ({
      oldName,
      newName,
    }: {
      oldName: string
      newName: string
    }) => {
      await api.post('/api/admin/rule-templates/rename', {
        old_name: oldName,
        new_name: newName,
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['rule-templates'] })
      toast.success('模板重命名成功')
      setIsRenameDialogOpen(false)
      setRenamingTemplate(null)
      setNewTemplateName('')
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '重命名失败')
    },
  })

  // Load template content when data is fetched
  useEffect(() => {
    if (templateData && isEditorOpen) {
      isInitLoadRef.current = true
      setTemplateContent(templateData)
      const vars = extractTemplateVariables(templateData)
      setTemplateVariables(vars)
      const groups = extractProxyGroups(templateData, vars)
      setProxyGroups(groups)
      // Auto-enable region proxy groups toggle if any group has includeRegionProxyGroups
      const hasRegionProxyGroups = groups.some(
        (g) => g.includeRegionProxyGroups
      )
      setEnableRegionProxyGroups(hasRegionProxyGroups)
      setIsDirty(false)
      // Allow ProxyGroupSelect's ensureMarkers setTimeout to finish before enabling dirty tracking
      setTimeout(() => {
        isInitLoadRef.current = false
        // Check for local draft
        if (editingTemplateName) {
          const draftJson = localStorage.getItem(
            TEMPLATE_DRAFT_KEY_PREFIX + editingTemplateName
          )
          if (draftJson) {
            try {
              const draft = JSON.parse(draftJson)
              // Normalize templateData the same way draft was saved
              const vars = extractTemplateVariables(templateData)
              const groups = extractProxyGroups(templateData, vars)
              const normalizedData =
                groups.length > 0
                  ? updateProxyGroups(templateData, groups)
                  : templateData
              if (draft.templateContent !== normalizedData) {
                pendingDraftRef.current = draft
                setIsDraftRecoveryOpen(true)
              } else {
                localStorage.removeItem(
                  TEMPLATE_DRAFT_KEY_PREFIX + editingTemplateName
                )
              }
            } catch {
              localStorage.removeItem(
                TEMPLATE_DRAFT_KEY_PREFIX + editingTemplateName
              )
            }
          }
        }
      }, 50)
    }
  }, [templateData, isEditorOpen])

  // Auto-refresh proxy-groups preview when proxyGroups changes
  useEffect(() => {
    if (!isEditorOpen) return

    // Generate proxy-groups YAML preview locally (no API call needed)
    if (proxyGroups.length > 0) {
      const preview = generateProxyGroupsPreview(proxyGroups)
      setPreviewContent(preview)
    } else {
      setPreviewContent('')
    }
  }, [proxyGroups, isEditorOpen])

  // Write draft to localStorage when dirty
  useEffect(() => {
    if (!isDirty || !editingTemplateName || isInitLoadRef.current) return
    let content = templateContent
    if (editorTab === 'visual' && proxyGroups.length > 0) {
      content = updateProxyGroups(templateContent, proxyGroups)
    }
    const draft = {
      templateContent: content,
      proxyGroups,
      enableRegionProxyGroups,
      templateVariables,
      editorTab,
      savedAt: Date.now(),
    }
    localStorage.setItem(
      TEMPLATE_DRAFT_KEY_PREFIX + editingTemplateName,
      JSON.stringify(draft)
    )
  }, [
    isDirty,
    templateContent,
    proxyGroups,
    enableRegionProxyGroups,
    templateVariables,
    editorTab,
    editingTemplateName,
  ])

  // Sync proxy groups to YAML when switching tabs
  const syncProxyGroupsToYaml = useCallback(() => {
    if (proxyGroups.length > 0) {
      const newContent = updateProxyGroups(templateContent, proxyGroups)
      setTemplateContent(newContent)
    }
  }, [proxyGroups, templateContent])

  // Handle tab change
  const handleTabChange = (tab: string) => {
    if (editorTab === 'visual' && tab === 'yaml') {
      syncProxyGroupsToYaml()
    } else if (editorTab === 'yaml' && tab === 'visual') {
      const vars = extractTemplateVariables(templateContent)
      setTemplateVariables(vars)
      setProxyGroups(extractProxyGroups(templateContent, vars))
    }
    setEditorTab(tab as 'visual' | 'yaml')
  }

  // Handle edit
  const handleEdit = (name: string) => {
    setEditingTemplateName(name)
    setIsEditorOpen(true)
    setEditorTab('visual')
    setPreviewContent('')
  }

  // Handle delete
  const handleDelete = (name: string) => {
    setDeletingTemplateName(name)
    setIsDeleteDialogOpen(true)
  }

  // Handle rename
  const handleRename = (name: string) => {
    setRenamingTemplate(name)
    setNewTemplateName(name)
    setIsRenameDialogOpen(true)
  }

  // Handle list preview (eye button in table)
  const handleListPreview = async (name: string) => {
    setListPreviewTemplateName(name)
    setListPreviewOpen(true)
    setListPreviewLoading(true)
    setListPreviewContent('')
    setListPreviewTemplateContent('')

    try {
      // Fetch template content
      const templateResponse = await api.get(
        `/api/admin/rule-templates/${encodeURIComponent(name)}`
      )
      const content = templateResponse.data.content
      setListPreviewTemplateContent(content)

      // Fetch nodes for preview
      const nodesResponse = await api.get('/api/admin/nodes')
      const nodes = (nodesResponse.data.nodes || [])
        .map((node: any) => {
          if (node.clash_config) {
            try {
              return JSON.parse(node.clash_config)
            } catch {
              return { name: node.node_name, type: node.protocol }
            }
          }
          return { name: node.node_name, type: node.protocol }
        })
        .filter((n: any) => n.name && n.type)

      // Generate preview
      const previewResponse = await api.post('/api/admin/template-v3/preview', {
        template_content: content,
        proxies: nodes,
      })
      setListPreviewContent(previewResponse.data.content)
    } catch (error: any) {
      toast.error(error.response?.data?.error || '预览生成失败')
      setListPreviewOpen(false)
    } finally {
      setListPreviewLoading(false)
    }
  }

  // Handle save
  const handleSave = () => {
    if (!editingTemplateName) return
    let content = templateContent
    if (editorTab === 'visual') {
      content = updateProxyGroups(templateContent, proxyGroups)
    }
    updateMutation.mutate({ name: editingTemplateName, content })
  }

  // Handle close editor
  const handleCloseEditor = () => {
    if (isDirty) {
      setIsCloseConfirmOpen(true)
      return
    }
    doCloseEditor()
  }

  const doCloseEditor = () => {
    setIsEditorOpen(false)
    setEditingTemplateName(null)
    setTemplateContent('')
    setProxyGroups([])
    setPreviewContent('')
    setIsDirty(false)
    setIsCloseConfirmOpen(false)
    setEnableRegionProxyGroups(false)
  }

  const handleRecoverDraft = () => {
    const draft = pendingDraftRef.current
    if (!draft) return
    isInitLoadRef.current = true
    setTemplateContent(draft.templateContent)
    setProxyGroups(draft.proxyGroups)
    setEnableRegionProxyGroups(draft.enableRegionProxyGroups)
    setTemplateVariables(draft.templateVariables)
    setEditorTab(draft.editorTab)
    setIsDirty(true)
    setTimeout(() => {
      isInitLoadRef.current = false
    }, 50)
    setIsDraftRecoveryOpen(false)
    pendingDraftRef.current = null
  }

  const handleDiscardDraft = () => {
    if (editingTemplateName) {
      localStorage.removeItem(TEMPLATE_DRAFT_KEY_PREFIX + editingTemplateName)
    }
    setIsDraftRecoveryOpen(false)
    pendingDraftRef.current = null
  }

  // Region proxy group names for checking
  const regionGroupNames = getRegionProxyGroupNames()

  // Handle region proxy groups toggle
  const handleRegionProxyGroupsToggle = (enabled: boolean) => {
    setEnableRegionProxyGroups(enabled)
    setIsDirty(true)

    if (enabled) {
      // Add region proxy groups at the end
      const regionGroups = generateRegionProxyGroups('url-test')
      // Filter out any existing region groups to avoid duplicates
      const nonRegionGroups = proxyGroups.filter(
        (g) => !regionGroupNames.includes(g.name)
      )
      setProxyGroups([...nonRegionGroups, ...regionGroups])
    } else {
      // Remove region proxy groups and clear includeRegionProxyGroups from all groups
      const updatedGroups = proxyGroups
        .filter((g) => !regionGroupNames.includes(g.name))
        .map((g) => ({
          ...g,
          includeRegionProxyGroups: false,
          // Remove REGION_PROXY_GROUPS_MARKER from proxyOrder
          proxyOrder: g.proxyOrder.filter(
            (item) => item !== REGION_PROXY_GROUPS_MARKER
          ),
        }))
      setProxyGroups(updatedGroups)
    }
  }

  // Handle proxy group change
  const handleProxyGroupChange = (
    index: number,
    group: ProxyGroupFormState
  ) => {
    const newGroups = [...proxyGroups]
    newGroups[index] = group
    setProxyGroups(newGroups)
    if (!isInitLoadRef.current) {
      setIsDirty(true)
    }
  }

  // Handle proxy group delete
  const handleProxyGroupDelete = (index: number) => {
    setProxyGroups(proxyGroups.filter((_, i) => i !== index))
    setIsDirty(true)
  }

  // Handle proxy group move
  const handleProxyGroupMoveUp = (index: number) => {
    if (index === 0) return
    const newGroups = [...proxyGroups]
    ;[newGroups[index - 1], newGroups[index]] = [
      newGroups[index],
      newGroups[index - 1],
    ]
    setProxyGroups(newGroups)
    setIsDirty(true)
  }

  const handleProxyGroupMoveDown = (index: number) => {
    if (index === proxyGroups.length - 1) return
    const newGroups = [...proxyGroups]
    ;[newGroups[index], newGroups[index + 1]] = [
      newGroups[index + 1],
      newGroups[index],
    ]
    setProxyGroups(newGroups)
    setIsDirty(true)
  }

  // Handle add proxy group
  const handleAddProxyGroup = () => {
    setProxyGroups([
      ...proxyGroups,
      createDefaultFormState(`新代理组 ${proxyGroups.length + 1}`),
    ])
    setIsDirty(true)
  }

  // Handle preview
  const handlePreview = async () => {
    setIsPreviewLoading(true)
    try {
      let content = templateContent
      if (editorTab === 'visual') {
        content = updateProxyGroups(templateContent, proxyGroups)
      }
      const response = await api.post('/api/admin/template-v3/preview', {
        template_content: content,
        proxies: nodesData || [],
      })
      setPreviewContent(response.data.content)
    } catch (error: any) {
      toast.error(error.response?.data?.error || '预览生成失败')
    } finally {
      setIsPreviewLoading(false)
    }
  }

  // Handle YAML content change
  const handleYamlChange = (value: string) => {
    setTemplateContent(value)
    setIsDirty(true)
  }

  // Replace markers with Chinese display names for preview
  const formatTemplateForDisplay = (content: string) => {
    return content
      .replace(new RegExp(PROXY_NODES_MARKER, 'g'), PROXY_NODES_DISPLAY)
      .replace(new RegExp(PROXY_PROVIDERS_MARKER, 'g'), PROXY_PROVIDERS_DISPLAY)
      .replace(
        new RegExp(REGION_PROXY_GROUPS_MARKER, 'g'),
        REGION_PROXY_GROUPS_DISPLAY
      )
  }

  // Table columns
  const columns: DataTableColumn<string>[] = [
    {
      header: '模板名称',
      cell: (name) => <span className='font-medium'>{name}</span>,
    },
    {
      header: '操作',
      cell: (name) => (
        <div className='flex items-center gap-1'>
          <Button
            variant='ghost'
            size='icon'
            onClick={() => handleEdit(name)}
            title='编辑'
          >
            <Pencil className='h-4 w-4' />
          </Button>
          <Button
            variant='ghost'
            size='icon'
            onClick={() => handleListPreview(name)}
            title='预览'
          >
            <Eye className='h-4 w-4' />
          </Button>
          <Button
            variant='ghost'
            size='icon'
            onClick={() => handleDelete(name)}
            title='删除'
          >
            <Trash2 className='text-destructive h-4 w-4' />
          </Button>
        </div>
      ),
    },
  ]

  return (
    <div className='bg-background min-h-svh'>
      <main className='mx-auto w-full max-w-7xl px-4 py-8 sm:px-6'>
        <Card>
          <CardHeader className='flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between'>
            <div>
              <CardTitle>V3 模板管理</CardTitle>
              <CardDescription>
                管理 mihomo 风格的规则模板，支持 include-all、filter 等高级特性
              </CardDescription>
            </div>
            <Button
              onClick={() => setIsUploadDialogOpen(true)}
              className='w-full sm:w-auto'
            >
              <Plus className='mr-2 h-4 w-4' />
              新建模板
            </Button>
          </CardHeader>
          <CardContent>
            <DataTable
              columns={columns}
              data={templates}
              getRowKey={(name) => name}
              emptyText='暂无模板，点击上方按钮创建'
              mobileCard={{
                header: (name) => (
                  <span className='text-base font-medium'>{name}</span>
                ),
                actions: (name) => (
                  <div className='flex w-full items-center justify-between gap-4 px-2'>
                    <Button
                      variant='ghost'
                      size='sm'
                      onClick={() => handleEdit(name)}
                      className='flex-1'
                    >
                      <Pencil className='mr-1.5 h-4 w-4' /> 编辑
                    </Button>
                    <Button
                      variant='ghost'
                      size='sm'
                      onClick={() => handleListPreview(name)}
                      className='flex-1'
                    >
                      <Eye className='mr-1.5 h-4 w-4' /> 预览
                    </Button>
                    <Button
                      variant='ghost'
                      size='sm'
                      onClick={() => handleDelete(name)}
                      className='text-destructive hover:text-destructive hover:bg-destructive/10 flex-1'
                    >
                      <Trash2 className='mr-1.5 h-4 w-4' /> 删除
                    </Button>
                  </div>
                ),
              }}
            />
          </CardContent>
        </Card>

        {/* Editor Dialog */}
        <Dialog
          open={isEditorOpen}
          onOpenChange={(open) => !open && handleCloseEditor()}
        >
          <DialogContent
            className={cn(
              'flex h-[90vh] flex-col',
              isMobile
                ? '!w-[95vw] !max-w-[95vw] p-4'
                : '!w-[85vw] !max-w-[85vw]'
            )}
            showCloseButton={false}
          >
            <DialogHeader className='flex-shrink-0'>
              <div
                className={cn(
                  'flex justify-between gap-4',
                  isMobile ? 'flex-col items-start' : 'items-center'
                )}
              >
                <div>
                  <DialogTitle className='break-all'>
                    {editingTemplateName}
                  </DialogTitle>
                  <DialogDescription>编辑模板配置</DialogDescription>
                </div>
                <div
                  className={cn(
                    'flex items-center gap-2',
                    isMobile ? 'w-full justify-between' : ''
                  )}
                >
                  {isDirty && <Badge variant='secondary'>未保存</Badge>}
                  <div className='flex gap-2'>
                    <Button
                      onClick={handleSave}
                      disabled={updateMutation.isPending}
                      size={isMobile ? 'sm' : 'default'}
                    >
                      <Save className='mr-1 h-4 w-4 sm:mr-2' />
                      保存
                    </Button>
                    <Button
                      variant='outline'
                      onClick={handleCloseEditor}
                      size={isMobile ? 'sm' : 'default'}
                    >
                      关闭
                    </Button>
                  </div>
                </div>
              </div>
            </DialogHeader>

            {/* Mobile: Preview below save button */}
            {isMobile && (
              <div className='mt-2 flex-shrink-0 border-b pb-4'>
                <Collapsible
                  open={isPreviewOpen}
                  onOpenChange={setIsPreviewOpen}
                >
                  <CollapsibleTrigger asChild>
                    <Button variant='outline' className='h-8 w-full text-sm'>
                      {isPreviewOpen ? '收起配置预览' : '展开配置预览'}
                    </Button>
                  </CollapsibleTrigger>
                  <CollapsibleContent className='mt-4 h-[250px]'>
                    <TemplatePreview
                      content={previewContent}
                      isLoading={isPreviewLoading}
                      onRefresh={handlePreview}
                      title='代理组配置'
                      className='h-full'
                    />
                  </CollapsibleContent>
                </Collapsible>
              </div>
            )}

            <div
              className={cn(
                'mt-4 flex flex-1 gap-4 overflow-hidden',
                isMobile ? 'flex-col' : 'flex-row'
              )}
            >
              {/* Editor Panel - Left column on tablet/desktop */}
              <div
                className={cn(
                  'flex flex-col overflow-hidden',
                  isMobile ? 'w-full flex-1' : isTablet ? 'w-[55%]' : 'w-[40%]'
                )}
              >
                <Tabs
                  value={editorTab}
                  onValueChange={handleTabChange}
                  className='flex h-full flex-col overflow-hidden'
                >
                  <TabsList className='grid w-full flex-shrink-0 grid-cols-2'>
                    <TabsTrigger value='visual'>可视化编辑</TabsTrigger>
                    <TabsTrigger value='yaml'>YAML 代码</TabsTrigger>
                  </TabsList>

                  <TabsContent
                    value='visual'
                    className='mt-4 flex min-h-0 flex-1 flex-col overflow-hidden data-[state=inactive]:hidden'
                  >
                    <ScrollArea className='h-full flex-1'>
                      <div className='space-y-3 pr-3 pb-4'>
                        {/* Region Proxy Groups Toggle */}
                        <div className='bg-muted/30 flex items-center justify-between rounded-lg border p-3'>
                          <div className='flex flex-col gap-1 sm:flex-row sm:items-center sm:gap-2'>
                            <Label
                              htmlFor='region-toggle'
                              className='font-medium'
                            >
                              开启区域代理组
                            </Label>
                            <span className='text-muted-foreground text-xs'>
                              自动添加按地区分类的代理组
                            </span>
                          </div>
                          <Switch
                            id='region-toggle'
                            checked={enableRegionProxyGroups}
                            onCheckedChange={handleRegionProxyGroupsToggle}
                          />
                        </div>

                        {proxyGroups.map((group, index) => (
                          <ProxyGroupEditor
                            key={index}
                            group={group}
                            index={index}
                            allGroupNames={proxyGroups.map((g) => g.name)}
                            onChange={handleProxyGroupChange}
                            onDelete={handleProxyGroupDelete}
                            onMoveUp={handleProxyGroupMoveUp}
                            onMoveDown={handleProxyGroupMoveDown}
                            isFirst={index === 0}
                            isLast={index === proxyGroups.length - 1}
                            showRegionToggle={enableRegionProxyGroups}
                            isRegionGroup={regionGroupNames.includes(
                              group.name
                            )}
                            variables={templateVariables}
                          />
                        ))}
                        <Button
                          variant='outline'
                          className='mt-2 w-full'
                          onClick={handleAddProxyGroup}
                        >
                          <Plus className='mr-2 h-4 w-4' />
                          添加代理组
                        </Button>
                      </div>
                    </ScrollArea>
                  </TabsContent>

                  <TabsContent
                    value='yaml'
                    className='mt-4 flex min-h-0 flex-1 flex-col overflow-hidden data-[state=inactive]:hidden'
                  >
                    <Textarea
                      value={templateContent}
                      onChange={(e) => handleYamlChange(e.target.value)}
                      className='flex-1 resize-none p-4 font-mono text-xs sm:text-sm'
                      placeholder='YAML 内容...'
                    />
                  </TabsContent>
                </Tabs>
              </div>

              {/* Preview Panel - Right column(s) on tablet/desktop */}
              {!isMobile && (
                <div
                  className={cn(
                    'flex overflow-hidden border-l pl-4',
                    isTablet ? 'w-[45%]' : 'w-[60%]'
                  )}
                >
                  <TemplatePreview
                    content={previewContent}
                    isLoading={isPreviewLoading}
                    onRefresh={handlePreview}
                    className='h-full flex-1'
                    title='代理组配置'
                  />
                </div>
              )}
            </div>
          </DialogContent>
        </Dialog>

        {/* Upload Dialog */}
        <TemplateUploadDialog
          open={isUploadDialogOpen}
          onOpenChange={setIsUploadDialogOpen}
          onUpload={(file) => uploadMutation.mutate(file)}
          onCreate={(name, content) => createMutation.mutate({ name, content })}
          isLoading={uploadMutation.isPending || createMutation.isPending}
        />

        {/* List Preview Dialog */}
        <Dialog open={listPreviewOpen} onOpenChange={setListPreviewOpen}>
          <DialogContent
            className={cn(
              'flex h-[85vh] flex-col',
              isMobile
                ? '!w-[95vw] !max-w-[95vw] p-4'
                : '!w-[90vw] !max-w-[90vw]'
            )}
            showCloseButton={false}
          >
            <DialogHeader className='flex-shrink-0'>
              <div className='flex items-center justify-between'>
                <div>
                  <DialogTitle className='w-[200px] truncate break-all sm:w-auto'>
                    预览: {listPreviewTemplateName}
                  </DialogTitle>
                  <DialogDescription className='hidden sm:block'>
                    左侧为模板配置，右侧为最终订阅配置
                  </DialogDescription>
                </div>
                <Button
                  variant='outline'
                  onClick={() => setListPreviewOpen(false)}
                  size={isMobile ? 'sm' : 'default'}
                >
                  关闭
                </Button>
              </div>
            </DialogHeader>
            <div
              className={cn(
                'flex flex-1 gap-4 overflow-hidden',
                isMobile ? 'flex-col' : 'flex-row'
              )}
            >
              {listPreviewLoading ? (
                <div className='flex h-full w-full items-center justify-center'>
                  <span className='text-muted-foreground'>正在生成预览...</span>
                </div>
              ) : (
                <>
                  {/* Left: Template Config */}
                  <div
                    className={cn(
                      'flex flex-col overflow-hidden',
                      isMobile ? 'h-1/2 w-full' : 'w-1/2'
                    )}
                  >
                    <div className='text-muted-foreground mb-2 text-sm font-medium'>
                      模板配置
                    </div>
                    <Card className='flex-1 overflow-hidden'>
                      <ScrollArea className='h-full'>
                        <pre className='p-2 font-mono text-xs break-all whitespace-pre-wrap sm:p-4'>
                          {formatTemplateForDisplay(listPreviewTemplateContent)}
                        </pre>
                      </ScrollArea>
                    </Card>
                  </div>
                  {/* Right: Final Subscription Config */}
                  <div
                    className={cn(
                      'flex flex-col overflow-hidden',
                      isMobile ? 'h-1/2 w-full' : 'w-1/2'
                    )}
                  >
                    <div className='text-muted-foreground mb-2 text-sm font-medium'>
                      最终订阅配置
                    </div>
                    <Card className='flex-1 overflow-hidden'>
                      <ScrollArea className='h-full'>
                        <pre className='p-2 font-mono text-xs break-all whitespace-pre-wrap sm:p-4'>
                          {listPreviewContent}
                        </pre>
                      </ScrollArea>
                    </Card>
                  </div>
                </>
              )}
            </div>
          </DialogContent>
        </Dialog>

        {/* Delete Confirmation Dialog */}
        <AlertDialog
          open={isDeleteDialogOpen}
          onOpenChange={setIsDeleteDialogOpen}
        >
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>确认删除</AlertDialogTitle>
              <AlertDialogDescription>
                确定要删除模板 "{deletingTemplateName}" 吗？此操作无法撤销。
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>取消</AlertDialogCancel>
              <AlertDialogAction
                onClick={() =>
                  deletingTemplateName &&
                  deleteMutation.mutate(deletingTemplateName)
                }
                className='bg-destructive text-destructive-foreground hover:bg-destructive/90'
              >
                删除
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>

        {/* Rename Dialog */}
        <Dialog open={isRenameDialogOpen} onOpenChange={setIsRenameDialogOpen}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>重命名模板</DialogTitle>
              <DialogDescription>输入新的模板名称</DialogDescription>
            </DialogHeader>
            <div className='py-4'>
              <Input
                value={newTemplateName}
                onChange={(e) => setNewTemplateName(e.target.value)}
                placeholder='新模板名称'
              />
            </div>
            <DialogFooter>
              <Button
                variant='outline'
                onClick={() => setIsRenameDialogOpen(false)}
              >
                取消
              </Button>
              <Button
                onClick={() =>
                  renamingTemplate &&
                  renameMutation.mutate({
                    oldName: renamingTemplate,
                    newName: newTemplateName,
                  })
                }
                disabled={renameMutation.isPending || !newTemplateName.trim()}
              >
                确认
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        {/* Close Confirmation Dialog */}
        <AlertDialog
          open={isCloseConfirmOpen}
          onOpenChange={setIsCloseConfirmOpen}
        >
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>确认关闭</AlertDialogTitle>
              <AlertDialogDescription>
                有未保存的更改，确定要关闭吗？
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>取消</AlertDialogCancel>
              <AlertDialogAction onClick={doCloseEditor}>
                确定关闭
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>

        {/* Draft Recovery Dialog */}
        <AlertDialog
          open={isDraftRecoveryOpen}
          onOpenChange={setIsDraftRecoveryOpen}
        >
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>恢复本地缓存</AlertDialogTitle>
              <AlertDialogDescription>
                检测到未保存的本地缓存，是否恢复？
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel onClick={handleDiscardDraft}>
                放弃
              </AlertDialogCancel>
              <AlertDialogAction onClick={handleRecoverDraft}>
                恢复
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </main>
    </div>
  )
}
