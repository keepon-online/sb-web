import { useState, useMemo } from 'react'
import {
  DndContext,
  PointerSensor,
  TouchSensor,
  useSensor,
  useSensors,
  closestCenter,
  type DragEndEvent,
} from '@dnd-kit/core'
import {
  SortableContext,
  useSortable,
  verticalListSortingStrategy,
  arrayMove,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import {
  ChevronDown,
  ChevronUp,
  Search,
  X,
  Edit2,
  Check,
  Plus,
  Settings2,
  GripVertical,
} from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetFooter,
} from '@/components/ui/sheet'
import { Twemoji } from '@/components/twemoji'

interface ProxyGroup {
  name: string
  type: string
  proxies: string[]
  url?: string
  interval?: number
  strategy?: 'round-robin' | 'consistent-hashing' | 'sticky-sessions'
  use?: string[]
  dialerProxyGroup?: string
}

interface Node {
  node_name: string
  tag?: string
  tags?: string[]
  [key: string]: any
}

// 特殊节点列表
const SPECIAL_NODES = ['♻️ 自动选择', '🚀 节点选择', 'DIRECT', 'REJECT']

interface MobileEditNodesDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  proxyGroups: ProxyGroup[]
  availableNodes: string[]
  allNodes: Node[]
  onProxyGroupsChange: (groups: ProxyGroup[]) => void
  onSave: () => void
  onRemoveNodeFromGroup: (groupName: string, nodeIndex: number) => void
  onRemoveGroup: (groupName: string) => void
  onRenameGroup: (oldName: string, newName: string) => void
  showSpecialNodesAtBottom?: boolean // 是否在底部显示特殊节点
  proxyProviderConfigs?: Array<{ id: number; name: string }> // 代理集合配置列表
}

// 可排序节点组件
interface SortableNodeItemProps {
  id: string
  proxy: string
  onRemove: () => void
}

function SortableNodeItem({ id, proxy, onRemove }: SortableNodeItemProps) {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({
    id,
    // 禁用拖拽结束时的动画，避免节点先回原位再移动
    animateLayoutChanges: () => false,
  })

  const style = {
    transform: CSS.Transform.toString(transform),
    transition: isDragging ? undefined : transition,
  }

  return (
    <div
      ref={setNodeRef}
      style={style}
      className={`hover:bg-accent flex items-center justify-between gap-2 rounded border p-2 ${
        isDragging ? 'bg-background z-50 opacity-50 shadow-lg' : ''
      }`}
    >
      <div
        {...attributes}
        {...listeners}
        className='-ml-1 cursor-grab touch-none p-1 active:cursor-grabbing'
      >
        <GripVertical className='text-muted-foreground h-4 w-4' />
      </div>
      <span className='flex-1 truncate text-sm'>
        <Twemoji>{proxy}</Twemoji>
      </span>
      <Button
        variant='ghost'
        size='sm'
        className='h-6 w-6 shrink-0 p-0'
        onClick={onRemove}
      >
        <X className='text-muted-foreground hover:text-destructive h-4 w-4' />
      </Button>
    </div>
  )
}

export function MobileEditNodesDialog({
  open,
  onOpenChange,
  proxyGroups,
  availableNodes: _availableNodes,
  allNodes,
  onProxyGroupsChange,
  onSave,
  onRemoveNodeFromGroup,
  onRemoveGroup,
  onRenameGroup,
  showSpecialNodesAtBottom = false,
  proxyProviderConfigs = [],
}: MobileEditNodesDialogProps) {
  const [expandedGroups, setExpandedGroups] = useState<Set<string>>(new Set())
  const [editingGroupName, setEditingGroupName] = useState<string | null>(null)
  const [editingGroupNewName, setEditingGroupNewName] = useState('')
  const [editSheetOpen, setEditSheetOpen] = useState(false)
  const [currentEditingGroup, setCurrentEditingGroup] = useState<string | null>(
    null
  )
  const [searchQuery, setSearchQuery] = useState('')
  const [selectedTag, setSelectedTag] = useState<string>('all')
  const [typePopoverGroup, setTypePopoverGroup] = useState<string | null>(null)

  // 配置传感器 - 支持触摸和指针
  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 8,
      },
    }),
    useSensor(TouchSensor, {
      activationConstraint: {
        delay: 200,
        tolerance: 8,
      },
    })
  )

  // 处理拖拽结束
  const handleDragEnd = (event: DragEndEvent, groupName: string) => {
    const { active, over } = event

    if (!over || active.id === over.id) return

    const group = proxyGroups.find((g) => g.name === groupName)
    if (!group) return

    // 从 id 中提取索引 (格式: groupName-index)
    const activeIdStr = String(active.id)
    const overIdStr = String(over.id)
    const prefix = `${groupName}-`

    if (!activeIdStr.startsWith(prefix) || !overIdStr.startsWith(prefix)) return

    const oldIndex = parseInt(activeIdStr.slice(prefix.length), 10)
    const newIndex = parseInt(overIdStr.slice(prefix.length), 10)

    if (isNaN(oldIndex) || isNaN(newIndex) || oldIndex < 0 || newIndex < 0)
      return
    if (oldIndex >= group.proxies.length || newIndex >= group.proxies.length)
      return

    const newProxies = arrayMove(group.proxies, oldIndex, newIndex)
    const newGroups = proxyGroups.map((g) => {
      if (g.name === groupName) {
        return { ...g, proxies: newProxies }
      }
      return g
    })
    onProxyGroupsChange(newGroups)
  }

  // 获取所有标签
  const allTags = useMemo(() => {
    const tags = new Set<string>()
    allNodes.forEach((node) => {
      const nodeTags = node.tags?.length
        ? node.tags
        : node.tag
          ? [node.tag]
          : []
      for (const t of nodeTags) tags.add(t)
    })
    return Array.from(tags).sort()
  }, [allNodes])

  // 过滤可用节点：显示除当前代理组外的所有节点
  const filteredAvailableNodes = useMemo(() => {
    const currentGroup = currentEditingGroup
      ? proxyGroups.find((g) => g.name === currentEditingGroup)
      : null
    const currentProxies = new Set(currentGroup?.proxies || [])

    return allNodes
      .filter((node) => {
        // 排除当前代理组已有的节点
        if (currentProxies.has(node.node_name)) return false
        // 搜索过滤
        if (
          searchQuery &&
          !node.node_name.toLowerCase().includes(searchQuery.toLowerCase())
        )
          return false
        // 标签过滤
        if (selectedTag !== 'all') {
          const nodeTags = node.tags?.length
            ? node.tags
            : node.tag
              ? [node.tag]
              : []
          if (!nodeTags.includes(selectedTag)) return false
        }
        return true
      })
      .map((n) => n.node_name)
  }, [allNodes, proxyGroups, currentEditingGroup, searchQuery, selectedTag])

  // 切换分组展开/折叠
  const toggleGroup = (groupName: string) => {
    const newExpanded = new Set(expandedGroups)
    if (newExpanded.has(groupName)) {
      newExpanded.delete(groupName)
    } else {
      newExpanded.add(groupName)
    }
    setExpandedGroups(newExpanded)
  }

  // 开始编辑分组名称
  const startEditGroupName = (groupName: string) => {
    setEditingGroupName(groupName)
    setEditingGroupNewName(groupName)
  }

  // 确认重命名
  const confirmRename = () => {
    if (
      editingGroupName &&
      editingGroupNewName.trim() &&
      editingGroupNewName !== editingGroupName
    ) {
      onRenameGroup(editingGroupName, editingGroupNewName.trim())
    }
    setEditingGroupName(null)
    setEditingGroupNewName('')
  }

  // 取消重命名
  const cancelRename = () => {
    setEditingGroupName(null)
    setEditingGroupNewName('')
  }

  // 打开编辑抽屉
  const openEditSheet = (groupName: string) => {
    setCurrentEditingGroup(groupName)
    setEditSheetOpen(true)
    setSearchQuery('')
    setSelectedTag('all')
  }

  // 关闭编辑抽屉
  const closeEditSheet = () => {
    setEditSheetOpen(false)
    setCurrentEditingGroup(null)
    setSearchQuery('')
    setSelectedTag('all')
  }

  // 检查节点是否在当前编辑的组中
  const isNodeInCurrentGroup = (nodeName: string) => {
    if (!currentEditingGroup) return false
    const group = proxyGroups.find((g) => g.name === currentEditingGroup)
    return group?.proxies.includes(nodeName) || false
  }

  // 检查代理集合是否在当前编辑的组中
  const isProviderInCurrentGroup = (providerName: string) => {
    if (!currentEditingGroup) return false
    const group = proxyGroups.find((g) => g.name === currentEditingGroup)
    return group?.use?.includes(providerName) || false
  }

  // 切换节点选中状态
  const toggleNodeInGroup = (nodeName: string) => {
    if (!currentEditingGroup) return

    const groupIndex = proxyGroups.findIndex(
      (g) => g.name === currentEditingGroup
    )
    if (groupIndex === -1) return

    const newGroups = [...proxyGroups]
    const group = newGroups[groupIndex]
    const nodeIndex = group.proxies.indexOf(nodeName)

    if (nodeIndex > -1) {
      // 移除节点
      group.proxies = group.proxies.filter((_, idx) => idx !== nodeIndex)
    } else {
      // 添加节点
      group.proxies = [...group.proxies, nodeName]
    }

    onProxyGroupsChange(newGroups)
  }

  // 切换代理集合选中状态
  const toggleProviderInGroup = (providerName: string) => {
    if (!currentEditingGroup) return

    const groupIndex = proxyGroups.findIndex(
      (g) => g.name === currentEditingGroup
    )
    if (groupIndex === -1) return

    const newGroups = [...proxyGroups]
    const group = newGroups[groupIndex]
    const useArray = group.use || []
    const providerIndex = useArray.indexOf(providerName)

    if (providerIndex > -1) {
      // 移除代理集合
      group.use = useArray.filter((_, idx) => idx !== providerIndex)
      if (group.use.length === 0) {
        delete group.use
      }
    } else {
      // 添加代理集合
      group.use = [...useArray, providerName]
    }

    onProxyGroupsChange(newGroups)
  }

  // 添加新代理组
  const addNewGroup = () => {
    const newGroupName = `新分组 ${proxyGroups.length + 1}`
    const newGroup: ProxyGroup = {
      name: newGroupName,
      type: 'select',
      proxies: [],
    }
    onProxyGroupsChange([...proxyGroups, newGroup])
    setExpandedGroups(new Set([...expandedGroups, newGroupName]))
  }

  // 代理组类型配置
  const proxyTypes = [
    { value: 'select', label: '手动选择', hasUrl: false, hasStrategy: false },
    { value: 'url-test', label: '自动选择', hasUrl: true, hasStrategy: false },
    { value: 'fallback', label: '自动回退', hasUrl: true, hasStrategy: false },
    {
      value: 'load-balance',
      label: '负载均衡',
      hasUrl: true,
      hasStrategy: true,
    },
  ]

  // 处理代理组类型变更
  const handleGroupTypeChange = (groupName: string, newType: string) => {
    const typeConfig = proxyTypes.find((t) => t.value === newType)
    const newGroups = proxyGroups.map((g) => {
      if (g.name !== groupName) return g

      const updatedGroup: ProxyGroup = {
        ...g,
        type: newType,
      }

      if (typeConfig?.hasUrl) {
        updatedGroup.url = g.url || 'https://www.gstatic.com/generate_204'
        updatedGroup.interval = g.interval || 300
      } else {
        delete updatedGroup.url
        delete updatedGroup.interval
      }

      if (typeConfig?.hasStrategy) {
        updatedGroup.strategy = g.strategy || 'round-robin'
      } else {
        delete updatedGroup.strategy
      }

      return updatedGroup
    })
    onProxyGroupsChange(newGroups)
  }

  // 处理负载均衡策略变更
  const handleStrategyChange = (
    groupName: string,
    strategy: ProxyGroup['strategy']
  ) => {
    const newGroups = proxyGroups.map((g) => {
      if (g.name !== groupName) return g
      return { ...g, strategy }
    })
    onProxyGroupsChange(newGroups)
  }

  // 处理中转代理组变更
  const handleDialerProxyGroupChange = (groupName: string, value: string) => {
    const newGroups = proxyGroups.map((g) => {
      if (g.name !== groupName) return g
      const updated = { ...g }
      if (value === '__none__') {
        delete updated.dialerProxyGroup
      } else {
        updated.dialerProxyGroup = value
      }
      return updated
    })
    onProxyGroupsChange(newGroups)
  }

  // 获取类型显示名称
  const getTypeLabel = (type: string) => {
    return proxyTypes.find((t) => t.value === type)?.label || type
  }

  return (
    <>
      <Sheet open={open} onOpenChange={onOpenChange}>
        <SheetContent side='bottom' className='flex h-[90vh] flex-col p-4'>
          <SheetHeader className='shrink-0'>
            <SheetTitle>手动分组节点</SheetTitle>
            <SheetDescription>
              点击分组展开查看节点，点击编辑按钮添加或移除节点
            </SheetDescription>
          </SheetHeader>

          <div className='-mx-2 flex-1 overflow-y-auto px-2 pt-4'>
            <div className='space-y-3'>
              {proxyGroups.map((group) => (
                <Card key={group.name} className='overflow-hidden'>
                  <CardContent className='p-0'>
                    {/* 分组头部 */}
                    <div className='bg-muted/30 space-y-2 p-3'>
                      {/* 第一行：代理组名称、类型、节点数量、删除按钮 */}
                      <div className='flex items-center gap-2'>
                        {editingGroupName === group.name ? (
                          <div className='flex min-w-0 flex-1 items-center gap-1'>
                            <Input
                              value={editingGroupNewName}
                              onChange={(e) =>
                                setEditingGroupNewName(e.target.value)
                              }
                              className='h-7 text-sm'
                              autoFocus
                            />
                            <Button
                              variant='ghost'
                              size='sm'
                              className='h-6 w-6 p-0'
                              onClick={confirmRename}
                            >
                              <Check className='h-4 w-4 text-green-600' />
                            </Button>
                            <Button
                              variant='ghost'
                              size='sm'
                              className='h-6 w-6 p-0'
                              onClick={cancelRename}
                            >
                              <X className='h-4 w-4 text-red-600' />
                            </Button>
                          </div>
                        ) : (
                          <>
                            <span
                              className='min-w-0 flex-1 cursor-pointer text-sm font-medium'
                              onClick={() => toggleGroup(group.name)}
                            >
                              <Twemoji>{group.name}</Twemoji>
                            </span>
                            <Badge
                              variant='secondary'
                              className='shrink-0 text-xs'
                            >
                              {getTypeLabel(group.type)}
                            </Badge>
                            <Badge
                              variant='outline'
                              className='shrink-0 text-xs'
                            >
                              {group.proxies.length}
                              {(group.use?.length || 0) > 0 &&
                                `+${group.use?.length}`}
                            </Badge>
                            <Button
                              variant='outline'
                              size='sm'
                              className='text-destructive hover:text-destructive h-7 w-7 shrink-0 p-0'
                              onClick={() => onRemoveGroup(group.name)}
                            >
                              <X className='h-4 w-4' />
                            </Button>
                          </>
                        )}
                      </div>

                      {/* 第二行：操作按钮 */}
                      {editingGroupName !== group.name && (
                        <div className='flex items-center justify-between gap-1.5'>
                          <div className='flex items-center gap-1.5'>
                            <Button
                              variant='outline'
                              size='sm'
                              className='h-7 px-2 text-xs'
                              onClick={() => startEditGroupName(group.name)}
                            >
                              <Edit2 className='mr-1 h-3 w-3' />
                              重命名
                            </Button>
                            <Popover
                              open={typePopoverGroup === group.name}
                              onOpenChange={(open) =>
                                setTypePopoverGroup(open ? group.name : null)
                              }
                            >
                              <PopoverTrigger asChild>
                                <Button
                                  variant='outline'
                                  size='sm'
                                  className='h-7 px-2 text-xs'
                                  title='切换代理组类型'
                                >
                                  <Settings2 className='mr-1 h-3 w-3' />
                                  类型
                                </Button>
                              </PopoverTrigger>
                              <PopoverContent
                                className='w-48 p-2'
                                align='start'
                              >
                                <div className='space-y-1'>
                                  {proxyTypes.map(({ value, label }) => (
                                    <Button
                                      key={value}
                                      variant={
                                        group.type === value
                                          ? 'default'
                                          : 'ghost'
                                      }
                                      size='sm'
                                      className='w-full justify-start'
                                      onClick={() => {
                                        handleGroupTypeChange(group.name, value)
                                        if (value !== 'load-balance')
                                          setTypePopoverGroup(null)
                                      }}
                                    >
                                      {label}
                                    </Button>
                                  ))}

                                  {group.type === 'load-balance' && (
                                    <div className='border-t pt-2'>
                                      <p className='text-muted-foreground mb-1 text-xs'>
                                        策略
                                      </p>
                                      <Select
                                        value={group.strategy || 'round-robin'}
                                        onValueChange={(value) => {
                                          handleStrategyChange(
                                            group.name,
                                            value as ProxyGroup['strategy']
                                          )
                                          setTypePopoverGroup(null)
                                        }}
                                      >
                                        <SelectTrigger className='h-8 text-xs'>
                                          <SelectValue />
                                        </SelectTrigger>
                                        <SelectContent>
                                          <SelectItem value='round-robin'>
                                            轮询
                                          </SelectItem>
                                          <SelectItem value='consistent-hashing'>
                                            一致性哈希
                                          </SelectItem>
                                          <SelectItem value='sticky-sessions'>
                                            粘性会话
                                          </SelectItem>
                                        </SelectContent>
                                      </Select>
                                    </div>
                                  )}

                                  <div className='border-t pt-2'>
                                    <p className='text-muted-foreground mb-1 text-xs'>
                                      中转代理组
                                    </p>
                                    <Select
                                      value={
                                        group.dialerProxyGroup || '__none__'
                                      }
                                      onValueChange={(value) => {
                                        handleDialerProxyGroupChange(
                                          group.name,
                                          value
                                        )
                                        setTypePopoverGroup(null)
                                      }}
                                    >
                                      <SelectTrigger className='h-8 text-xs'>
                                        <SelectValue placeholder='无' />
                                      </SelectTrigger>
                                      <SelectContent>
                                        <SelectItem value='__none__'>
                                          无
                                        </SelectItem>
                                        {proxyGroups
                                          .filter((g) => g.name !== group.name)
                                          .map((g) => (
                                            <SelectItem
                                              key={g.name}
                                              value={g.name}
                                            >
                                              <Twemoji>{g.name}</Twemoji>
                                            </SelectItem>
                                          ))}
                                      </SelectContent>
                                    </Select>
                                  </div>
                                </div>
                              </PopoverContent>
                            </Popover>
                            <Button
                              variant='outline'
                              size='sm'
                              className='h-7 text-xs'
                              onClick={() => openEditSheet(group.name)}
                            >
                              添加节点
                            </Button>
                          </div>
                          <Button
                            variant='outline'
                            size='sm'
                            className='h-7 items-center px-2 text-xs'
                            onClick={() => toggleGroup(group.name)}
                          >
                            {expandedGroups.has(group.name) ? (
                              <>
                                <ChevronUp className='h-3 w-3' />
                              </>
                            ) : (
                              <>
                                <ChevronDown className='h-3 w-3' />
                              </>
                            )}
                          </Button>
                        </div>
                      )}
                    </div>

                    {/* 分组内容（展开时显示） */}
                    {expandedGroups.has(group.name) && (
                      <div className='space-y-1.5 p-3'>
                        {group.proxies.length === 0 &&
                        (group.use?.length || 0) === 0 ? (
                          <p className='text-muted-foreground py-2 text-center text-sm'>
                            暂无节点，点击"添加节点"按钮添加
                          </p>
                        ) : (
                          <>
                            <DndContext
                              sensors={sensors}
                              collisionDetection={closestCenter}
                              onDragEnd={(e) => handleDragEnd(e, group.name)}
                            >
                              <SortableContext
                                items={group.proxies.map(
                                  (_, idx) => `${group.name}-${idx}`
                                )}
                                strategy={verticalListSortingStrategy}
                              >
                                {group.proxies.map((proxy, idx) => (
                                  <SortableNodeItem
                                    key={`${group.name}-${idx}`}
                                    id={`${group.name}-${idx}`}
                                    proxy={proxy}
                                    onRemove={() =>
                                      onRemoveNodeFromGroup(group.name, idx)
                                    }
                                  />
                                ))}
                              </SortableContext>
                            </DndContext>
                            {/* 代理集合（use）显示 */}
                            {(group.use || []).map((providerName, idx) => (
                              <div
                                key={`use-${idx}`}
                                className='flex items-center justify-between gap-2 rounded border border-purple-300 bg-purple-50 p-2 dark:border-purple-700 dark:bg-purple-950/20'
                              >
                                <span className='flex-1 truncate text-sm text-purple-700 dark:text-purple-300'>
                                  📦 {providerName}
                                </span>
                                <Button
                                  variant='ghost'
                                  size='sm'
                                  className='h-6 w-6 shrink-0 p-0'
                                  onClick={() => {
                                    const newGroups = proxyGroups.map((g) => {
                                      if (g.name === group.name) {
                                        const newUse = (g.use || []).filter(
                                          (_, i) => i !== idx
                                        )
                                        return {
                                          ...g,
                                          use:
                                            newUse.length > 0
                                              ? newUse
                                              : undefined,
                                        }
                                      }
                                      return g
                                    })
                                    onProxyGroupsChange(newGroups)
                                  }}
                                >
                                  <X className='hover:text-destructive h-4 w-4 text-purple-400' />
                                </Button>
                              </div>
                            ))}
                          </>
                        )}
                      </div>
                    )}
                  </CardContent>
                </Card>
              ))}

              {/* 添加代理组按钮 */}
              <Button
                variant='outline'
                className='w-full'
                onClick={addNewGroup}
              >
                <Plus className='mr-2 h-4 w-4' />
                添加代理组
              </Button>
            </div>
          </div>

          <SheetFooter className='shrink-0 pt-4'>
            <Button variant='outline' onClick={() => onOpenChange(false)}>
              取消
            </Button>
            <Button
              onClick={() => {
                onSave()
                onOpenChange(false)
              }}
            >
              确定
            </Button>
          </SheetFooter>
        </SheetContent>
      </Sheet>

      {/* 编辑节点的底部抽屉 */}
      <Sheet open={editSheetOpen} onOpenChange={setEditSheetOpen}>
        <SheetContent side='bottom' className='flex h-[80vh] flex-col p-4'>
          <SheetHeader className='shrink-0'>
            <SheetTitle>编辑分组: {currentEditingGroup}</SheetTitle>
            <SheetDescription>选择要添加到此分组的节点</SheetDescription>
          </SheetHeader>

          <div className='flex flex-1 flex-col space-y-3 overflow-hidden'>
            {/* 搜索框 */}
            <div className='relative shrink-0'>
              <Search className='text-muted-foreground absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2' />
              <Input
                placeholder='搜索节点...'
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className='pl-9'
              />
            </div>

            {/* 标签过滤 */}
            {allTags.length > 0 && (
              <div className='flex shrink-0 flex-wrap gap-2'>
                <Badge
                  variant={selectedTag === 'all' ? 'default' : 'outline'}
                  className='cursor-pointer'
                  onClick={() => setSelectedTag('all')}
                >
                  全部
                </Badge>
                {allTags.map((tag) => (
                  <Badge
                    key={tag}
                    variant={selectedTag === tag ? 'default' : 'outline'}
                    className='cursor-pointer'
                    onClick={() => setSelectedTag(tag)}
                  >
                    {tag}
                  </Badge>
                ))}
              </div>
            )}

            {/* 节点列表 */}
            <div className='-mx-2 flex-1 overflow-y-auto px-2 pt-2'>
              <div className='space-y-2'>
                {filteredAvailableNodes.length === 0 &&
                proxyProviderConfigs.length === 0 &&
                !showSpecialNodesAtBottom ? (
                  <p className='text-muted-foreground py-8 text-center text-sm'>
                    {searchQuery || selectedTag !== 'all'
                      ? '没有找到匹配的节点'
                      : '暂无可用节点'}
                  </p>
                ) : (
                  <>
                    {filteredAvailableNodes.map((nodeName) => {
                      const node = allNodes.find(
                        (n) => n.node_name === nodeName
                      )
                      const isSelected = isNodeInCurrentGroup(nodeName)

                      return (
                        <div
                          key={nodeName}
                          className={`flex cursor-pointer items-center gap-3 rounded-lg border p-3 transition-colors ${
                            isSelected
                              ? 'bg-accent border-primary'
                              : 'hover:bg-accent/50'
                          }`}
                          onClick={() => toggleNodeInGroup(nodeName)}
                        >
                          <Checkbox
                            checked={isSelected}
                            onCheckedChange={() => toggleNodeInGroup(nodeName)}
                            onClick={(e) => e.stopPropagation()}
                          />
                          <div className='min-w-0 flex-1'>
                            <p className='truncate text-sm font-medium'>
                              <Twemoji>{nodeName}</Twemoji>
                            </p>
                            {(node?.tags?.length
                              ? node.tags
                              : node?.tag
                                ? [node.tag]
                                : []
                            ).map((t) => (
                              <Badge
                                key={t}
                                variant='secondary'
                                className='mt-1 text-xs'
                              >
                                {t}
                              </Badge>
                            ))}
                          </div>
                        </div>
                      )
                    })}

                    {/* 代理集合区块 */}
                    {proxyProviderConfigs.length > 0 && (
                      <>
                        <div className='mt-3 border-t pt-3 pb-1'>
                          <span className='text-xs font-medium text-purple-600 dark:text-purple-400'>
                            📦 代理集合
                          </span>
                        </div>
                        {proxyProviderConfigs.map((config) => {
                          const isSelected = isProviderInCurrentGroup(
                            config.name
                          )
                          return (
                            <div
                              key={`provider-${config.id}`}
                              className={`flex cursor-pointer items-center gap-3 rounded-lg border p-3 transition-colors ${
                                isSelected
                                  ? 'border-purple-500 bg-purple-100 dark:bg-purple-950/40'
                                  : 'border-purple-300 bg-purple-50 hover:bg-purple-100 dark:border-purple-700 dark:bg-purple-950/20 dark:hover:bg-purple-900/30'
                              }`}
                              onClick={() => toggleProviderInGroup(config.name)}
                            >
                              <Checkbox
                                checked={isSelected}
                                onCheckedChange={() =>
                                  toggleProviderInGroup(config.name)
                                }
                                onClick={(e) => e.stopPropagation()}
                                className='border-purple-400 data-[state=checked]:border-purple-600 data-[state=checked]:bg-purple-600'
                              />
                              <div className='min-w-0 flex-1'>
                                <p className='truncate text-sm font-medium text-purple-700 dark:text-purple-300'>
                                  📦 {config.name}
                                </p>
                              </div>
                            </div>
                          )
                        })}
                      </>
                    )}

                    {/* 特殊节点区块 */}
                    {showSpecialNodesAtBottom && (
                      <>
                        <div className='mt-3 border-t pt-3 pb-1'>
                          <span className='text-muted-foreground text-xs font-medium'>
                            特殊节点
                          </span>
                        </div>
                        {SPECIAL_NODES.map((nodeName) => {
                          const isSelected = isNodeInCurrentGroup(nodeName)
                          return (
                            <div
                              key={`special-${nodeName}`}
                              className={`flex cursor-pointer items-center gap-3 rounded-lg border p-3 transition-colors ${
                                isSelected
                                  ? 'bg-accent border-primary'
                                  : 'hover:bg-accent/50'
                              }`}
                              onClick={() => toggleNodeInGroup(nodeName)}
                            >
                              <Checkbox
                                checked={isSelected}
                                onCheckedChange={() =>
                                  toggleNodeInGroup(nodeName)
                                }
                                onClick={(e) => e.stopPropagation()}
                              />
                              <div className='min-w-0 flex-1'>
                                <p className='truncate text-sm font-medium'>
                                  <Twemoji>{nodeName}</Twemoji>
                                </p>
                              </div>
                            </div>
                          )
                        })}
                      </>
                    )}
                  </>
                )}
              </div>
            </div>
          </div>

          <SheetFooter className='shrink-0'>
            <div className='flex w-full items-center justify-between'>
              <span className='text-muted-foreground text-sm'>
                已选择{' '}
                {proxyGroups.find((g) => g.name === currentEditingGroup)
                  ?.proxies.length || 0}{' '}
                个节点
                {(proxyGroups.find((g) => g.name === currentEditingGroup)?.use
                  ?.length || 0) > 0 && (
                  <span className='text-purple-600 dark:text-purple-400'>
                    {' '}
                    +{' '}
                    {
                      proxyGroups.find((g) => g.name === currentEditingGroup)
                        ?.use?.length
                    }{' '}
                    个集合
                  </span>
                )}
              </span>
              <Button onClick={closeEditSheet}>完成</Button>
            </div>
          </SheetFooter>
        </SheetContent>
      </Sheet>
    </>
  )
}
