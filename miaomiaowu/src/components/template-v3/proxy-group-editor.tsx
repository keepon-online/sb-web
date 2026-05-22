import { useState } from 'react'
import {
  ChevronDown,
  ChevronUp,
  Trash2,
  GripVertical,
  Link2,
  Variable,
  EyeOff,
} from 'lucide-react'
import {
  PROXY_GROUP_TYPES,
  hasProxyNodes,
  hasProxyProviders,
  type ProxyGroupFormState,
  type ProxyGroupType,
} from '@/lib/template-v3-utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { KeywordFilterInput } from './keyword-filter-input'
import { ProxyGroupSelect } from './proxy-group-select'
import { ProxyTypeSelect } from './proxy-type-select'

interface ProxyGroupEditorProps {
  group: ProxyGroupFormState
  index: number
  allGroupNames: string[]
  onChange: (index: number, group: ProxyGroupFormState) => void
  onDelete: (index: number) => void
  onMoveUp?: (index: number) => void
  onMoveDown?: (index: number) => void
  isFirst?: boolean
  isLast?: boolean
  showRegionToggle?: boolean
  isRegionGroup?: boolean
  variables?: Record<string, string> // 模板自定义变量
}

const GROUP_TYPE_LABELS: Record<ProxyGroupType, string> = {
  select: '手动选择',
  'url-test': '自动测速',
  fallback: '故障转移',
  'load-balance': '负载均衡',
  relay: '链式代理',
}

export function ProxyGroupEditor({
  group,
  index,
  allGroupNames,
  onChange,
  onDelete,
  onMoveUp,
  onMoveDown,
  isFirst = false,
  isLast = false,
  showRegionToggle = true,
  isRegionGroup = false,
  variables,
}: ProxyGroupEditorProps) {
  const [isOpen, setIsOpen] = useState(false)
  const [showRelayPicker, setShowRelayPicker] = useState(false)

  const updateField = <K extends keyof ProxyGroupFormState>(
    field: K,
    value: ProxyGroupFormState[K]
  ) => {
    onChange(index, { ...group, [field]: value })
  }

  const needsUrlTestOptions = ['url-test', 'fallback', 'load-balance'].includes(
    group.type
  )

  return (
    <Collapsible open={isOpen} onOpenChange={setIsOpen}>
      <div className={`rounded-lg border ${group.hidden ? 'opacity-60' : ''}`}>
        <CollapsibleTrigger asChild>
          <div className='hover:bg-accent/50 flex cursor-pointer flex-col justify-between gap-3 p-3 sm:flex-row sm:items-center sm:gap-0'>
            <div className='flex w-full flex-wrap items-center justify-end gap-2 sm:w-auto sm:gap-3'>
              <GripVertical className='text-muted-foreground h-4 w-4 shrink-0' />
              {group.icon &&
                (/^https?:\/\//.test(group.icon) ? (
                  <img
                    src={group.icon}
                    alt=''
                    className='h-5 w-5 shrink-0 object-contain'
                  />
                ) : (
                  <span className='shrink-0 text-base leading-none'>
                    {group.icon}
                  </span>
                ))}
              <span className='mr-auto max-w-[150px] truncate font-medium sm:max-w-none'>
                {group.name}
              </span>
              <Badge variant='outline' className='shrink-0 text-xs'>
                {GROUP_TYPE_LABELS[group.type]}
              </Badge>
              {group.hidden && (
                <Badge variant='secondary' className='shrink-0 gap-1 text-xs'>
                  <EyeOff className='h-3 w-3' />
                  隐藏
                </Badge>
              )}
              {group.filterKeywords && (
                <Badge variant='secondary' className='shrink-0 text-xs'>
                  有过滤
                </Badge>
              )}
            </div>
            <div className='flex w-full items-center justify-end gap-1 border-t pt-2 sm:w-auto sm:border-0 sm:pt-0'>
              {group.dialerProxyGroup && (
                <Badge
                  variant='secondary'
                  className='hover:bg-secondary/80 mr-auto max-w-[100px] shrink-0 cursor-pointer truncate text-xs sm:max-w-[150px]'
                  onClick={(e) => {
                    e.stopPropagation()
                    setShowRelayPicker(!showRelayPicker)
                  }}
                >
                  中转: {group.dialerProxyGroup}
                </Badge>
              )}
              <Button
                variant='ghost'
                size='icon'
                className={`h-8 w-8 ${group.dialerProxyGroup ? 'text-primary' : 'text-muted-foreground'}`}
                title={
                  group.dialerProxyGroup
                    ? `中转: ${group.dialerProxyGroup}`
                    : '设置中转代理组'
                }
                onClick={(e) => {
                  e.stopPropagation()
                  setShowRelayPicker(!showRelayPicker)
                }}
              >
                <Link2 className='h-4 w-4' />
              </Button>
              {onMoveUp && !isFirst && (
                <Button
                  variant='ghost'
                  size='icon'
                  className='h-8 w-8'
                  onClick={(e) => {
                    e.stopPropagation()
                    onMoveUp(index)
                  }}
                >
                  <ChevronUp className='h-4 w-4' />
                </Button>
              )}
              {onMoveDown && !isLast && (
                <Button
                  variant='ghost'
                  size='icon'
                  className='h-8 w-8'
                  onClick={(e) => {
                    e.stopPropagation()
                    onMoveDown(index)
                  }}
                >
                  <ChevronDown className='h-4 w-4' />
                </Button>
              )}
              <Button
                variant='ghost'
                size='icon'
                className='text-destructive h-8 w-8'
                onClick={(e) => {
                  e.stopPropagation()
                  onDelete(index)
                }}
              >
                <Trash2 className='h-4 w-4' />
              </Button>
              <ChevronDown
                className={`h-4 w-4 transition-transform ${isOpen ? 'rotate-180' : ''}`}
              />
            </div>
          </div>
        </CollapsibleTrigger>

        {showRelayPicker && (
          <div className='border-t px-3 pb-3'>
            <div className='flex items-center justify-between pt-3 pb-2'>
              <span className='text-muted-foreground text-xs'>
                选择中转代理组
              </span>
              {group.dialerProxyGroup && (
                <Badge
                  variant='outline'
                  className='hover:bg-destructive/10 hover:text-destructive cursor-pointer text-xs'
                  onClick={() => updateField('dialerProxyGroup', '')}
                >
                  清除
                </Badge>
              )}
            </div>
            <div className='flex flex-wrap gap-2'>
              {allGroupNames
                .filter((n) => n !== group.name)
                .map((n) => (
                  <Badge
                    key={n}
                    variant={
                      group.dialerProxyGroup === n ? 'default' : 'outline'
                    }
                    className={`cursor-pointer justify-center py-1.5 transition-colors ${
                      group.dialerProxyGroup === n ? '' : 'hover:bg-accent'
                    }`}
                    onClick={() =>
                      updateField(
                        'dialerProxyGroup',
                        group.dialerProxyGroup === n ? '' : n
                      )
                    }
                  >
                    {n}
                  </Badge>
                ))}
            </div>
          </div>
        )}

        <CollapsibleContent>
          <div className='space-y-4 overflow-hidden border-t p-4 pt-0'>
            {/* Row 1: Name and Type */}
            <div className='grid grid-cols-1 gap-4 sm:grid-cols-2'>
              <div className='space-y-2'>
                <Label>组名称</Label>
                <Input
                  value={group.name}
                  onChange={(e) => updateField('name', e.target.value)}
                  placeholder='代理组名称'
                  className='w-full'
                />
              </div>
              <div className='space-y-2'>
                <Label>组类型</Label>
                <Select
                  value={group.type}
                  onValueChange={(v) =>
                    updateField('type', v as ProxyGroupType)
                  }
                >
                  <SelectTrigger className='w-full'>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {PROXY_GROUP_TYPES.map((type) => (
                      <SelectItem key={type} value={type}>
                        {GROUP_TYPE_LABELS[type]}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>

            {/* Row 2: Include Options */}
            <div className='space-y-2'>
              <Label>节点来源</Label>
              <div className='flex flex-wrap gap-4'>
                <div className='flex items-center gap-2'>
                  <Switch
                    checked={group.includeAll}
                    onCheckedChange={(v) => {
                      onChange(index, {
                        ...group,
                        includeAll: v,
                        includeAllProxies: v,
                        includeAllProviders: v,
                      })
                    }}
                  />
                  <span className='text-sm'>代理集合+节点</span>
                </div>
                <div className='flex items-center gap-2'>
                  <Switch
                    checked={group.includeAllProxies}
                    onCheckedChange={(v) => {
                      const newIncludeAll = v && group.includeAllProviders
                      onChange(index, {
                        ...group,
                        includeAllProxies: v,
                        includeAll: v ? newIncludeAll : false,
                      })
                    }}
                  />
                  <span className='text-sm'>代理节点</span>
                </div>
                <div className='flex items-center gap-2'>
                  <Switch
                    checked={group.includeAllProviders}
                    onCheckedChange={(v) => {
                      const newIncludeAll = v && group.includeAllProxies
                      onChange(index, {
                        ...group,
                        includeAllProviders: v,
                        includeAll: v ? newIncludeAll : false,
                      })
                    }}
                  />
                  <span className='text-sm'>代理集合</span>
                </div>
                {showRegionToggle && !isRegionGroup && (
                  <div className='flex items-center gap-2'>
                    <Switch
                      checked={group.includeRegionProxyGroups}
                      onCheckedChange={(v) =>
                        updateField('includeRegionProxyGroups', v)
                      }
                    />
                    <span className='text-sm'>区域代理组</span>
                  </div>
                )}
              </div>
            </div>

            {/* Row 2.5: Proxy Order (groups, nodes, providers) */}
            <div className='w-full max-w-full overflow-hidden'>
              <ProxyGroupSelect
                label='代理顺序 (拖拽排序)'
                value={group.proxyOrder}
                onChange={(v) => updateField('proxyOrder', v)}
                availableGroups={allGroupNames.filter((n) => n !== group.name)}
                showNodesMarker={hasProxyNodes(group)}
                showProvidersMarker={hasProxyProviders(group)}
                showRegionGroupsMarker={group.includeRegionProxyGroups}
                placeholder='选择要引用的代理组'
              />
            </div>

            {/* 模板变量提示 */}
            {variables && Object.keys(variables).length > 0 && (
              <div className='flex flex-wrap items-center gap-2'>
                <TooltipProvider>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Badge
                        variant='outline'
                        className='shrink-0 cursor-help gap-1 border-dashed border-amber-500 text-xs text-amber-600 dark:text-amber-400'
                      >
                        <Variable className='h-3 w-3 shrink-0' />
                        模板变量 ({Object.keys(variables).length})
                      </Badge>
                    </TooltipTrigger>
                    <TooltipContent
                      side='bottom'
                      className='max-w-[90vw] break-all sm:max-w-md'
                    >
                      <div className='space-y-1 text-xs'>
                        {Object.entries(variables).map(([name, value]) => (
                          <div key={name} className='flex gap-2'>
                            <span className='shrink-0 font-mono font-semibold'>
                              {name}
                            </span>
                            <span className='truncate'>{value}</span>
                          </div>
                        ))}
                      </div>
                    </TooltipContent>
                  </Tooltip>
                </TooltipProvider>
              </div>
            )}

            {/* Row 3-4: Filter Keywords */}
            <div className='grid grid-cols-1 gap-4 sm:grid-cols-2'>
              <KeywordFilterInput
                label='筛选关键词 (filter)'
                value={group.filterKeywords}
                onChange={(v) => updateField('filterKeywords', v)}
                onVariableCleared={() =>
                  updateField('filterFromVariable', undefined)
                }
                placeholder='香港, HK, 港'
                description='匹配节点名称，用逗号分隔'
                fromVariable={group.filterFromVariable}
              />
              <KeywordFilterInput
                label='排除关键词 (exclude-filter)'
                value={group.excludeFilterKeywords}
                onChange={(v) => updateField('excludeFilterKeywords', v)}
                onVariableCleared={() =>
                  updateField('excludeFilterFromVariable', undefined)
                }
                placeholder='游戏, IPLC'
                description='排除匹配的节点'
                fromVariable={group.excludeFilterFromVariable}
              />
            </div>

            {/* Row 5: Type Filters */}
            <div className='grid grid-cols-1 gap-4 sm:grid-cols-2'>
              <ProxyTypeSelect
                label='包含类型 (include-type)'
                value={group.includeTypes}
                onChange={(v) => updateField('includeTypes', v)}
                placeholder='选择要包含的代理类型'
              />
              <ProxyTypeSelect
                label='排除类型 (exclude-type)'
                value={group.excludeTypes}
                onChange={(v) => updateField('excludeTypes', v)}
                placeholder='选择要排除的代理类型'
              />
            </div>

            {/* Row 6: URL Test Options */}
            {needsUrlTestOptions && (
              <div className='grid grid-cols-1 gap-4 sm:grid-cols-3'>
                <div className='space-y-2'>
                  <Label>测试 URL</Label>
                  <Input
                    value={group.url}
                    onChange={(e) => updateField('url', e.target.value)}
                    placeholder='https://www.gstatic.com/generate_204'
                    className='w-full'
                  />
                </div>
                <div className='space-y-2'>
                  <Label>测试间隔 (秒)</Label>
                  <Input
                    type='number'
                    value={group.interval}
                    onChange={(e) =>
                      updateField('interval', parseInt(e.target.value) || 300)
                    }
                    className='w-full'
                  />
                </div>
                {group.type !== 'load-balance' && (
                  <div className='space-y-2'>
                    <Label>容差 (ms)</Label>
                    <Input
                      type='number'
                      value={group.tolerance}
                      onChange={(e) =>
                        updateField('tolerance', parseInt(e.target.value) || 50)
                      }
                      className='w-full'
                    />
                  </div>
                )}
              </div>
            )}

            {/* Row 7: Icon and Hidden */}
            <div className='grid grid-cols-1 gap-4 sm:grid-cols-2'>
              <div className='space-y-2'>
                <Label>图标 (icon)</Label>
                <Input
                  value={group.icon}
                  onChange={(e) => updateField('icon', e.target.value)}
                  placeholder='URL 或 emoji'
                  className='w-full'
                />
              </div>
              <div className='flex items-center gap-2 sm:pt-8'>
                <Switch
                  checked={group.hidden}
                  onCheckedChange={(v) => updateField('hidden', v)}
                />
                <span className='text-sm'>隐藏此组 (hidden)</span>
              </div>
            </div>
          </div>
        </CollapsibleContent>
      </div>
    </Collapsible>
  )
}
