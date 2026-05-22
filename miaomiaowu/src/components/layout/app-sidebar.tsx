import { Link, useLocation } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import {
  Activity,
  Link as LinkIcon,
  Users,
  Files,
  Zap,
  Network,
  FileCode,
  Settings,
  FileStack,
  Shield,
  Rocket,
  Database,
  Share2,
  Cpu,
  ClipboardList,
} from 'lucide-react'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarGroupContent,
} from '@/components/ui/sidebar'
import { UserMenu } from './user-menu'
import { ThemeSwitch } from '@/components/theme-switch'
import { ColorThemeSelector } from '@/components/color-theme-selector'
import { useAuthStore } from '@/stores/auth-store'
import { profileQueryFn } from '@/lib/profile'
import { api } from '@/lib/api'

const baseNavLinks = [
  {
    title: '流量信息',
    to: '/',
    icon: Activity,
  },
  {
    title: '订阅链接',
    to: '/subscription',
    icon: LinkIcon,
  },
]

const adminNavLinks = [
  {
    title: '生成订阅',
    to: '/generator',
    icon: Zap,
  },
  {
    title: '节点管理',
    to: '/nodes',
    icon: Network,
  },
  {
    title: '订阅管理',
    to: '/subscribe-files',
    icon: Files,
  },
  {
    title: '模板管理',
    to: '/templates-v3',
    icon: FileStack,
  },
  {
    title: '规则管理',
    to: '/custom-rules',
    icon: FileCode,
  },
  {
    title: '用户管理',
    to: '/users',
    icon: Users,
  },
  {
    title: 'Sing-box',
    to: '/singbox',
    icon: Shield,
  },
  {
    title: 'Argo隧道',
    to: '/singbox/argo',
    icon: Rocket,
  },
  {
    title: 'WARP管理',
    to: '/singbox/warp',
    icon: Cpu,
  },
  {
    title: '系统优化',
    to: '/singbox/optimize',
    icon: Database,
  },
  {
    title: '订阅生成',
    to: '/singbox/subscription',
    icon: Zap,
  },
  {
    title: 'Git同步',
    to: '/singbox/sync',
    icon: Share2,
  },
  {
    title: '操作审计',
    to: '/audit',
    icon: ClipboardList,
  },
  {
    title: '系统设置',
    to: '/system-settings',
    icon: Settings,
  },
]

export function AppSidebar() {
  const { auth } = useAuthStore()
  const location = useLocation()

  const { data: profile } = useQuery({
    queryKey: ['profile'],
    queryFn: profileQueryFn,
    enabled: Boolean(auth.accessToken),
    staleTime: 5 * 60 * 1000,
  })

  const { data: userConfig } = useQuery({
    queryKey: ['user-config'],
    queryFn: async () => {
      const response = await api.get('/api/user/config')
      return response.data as { template_version: string }
    },
    enabled: Boolean(auth.accessToken),
    staleTime: 5 * 60 * 1000,
  })

  const isAdmin = Boolean(profile?.is_admin)
  const templateVersion = userConfig?.template_version || 'v2'

  const filteredAdminLinks = adminNavLinks.filter((link) => {
    if (link.to === '/templates-v3') {
      return templateVersion === 'v3'
    }
    return true
  })

  return (
    <Sidebar collapsible='icon'>
      <SidebarHeader className='flex flex-row items-center gap-3 px-4 py-4'>
        <div className='flex items-center gap-3 overflow-hidden'>
          <img
            src='/images/logo.webp'
            alt='Logo'
            className='size-8 rounded-lg border border-primary/20 shadow-sm'
          />
          <span className='font-bold text-primary whitespace-nowrap group-data-[collapsible=icon]:hidden'>
            妙妙屋
          </span>
        </div>
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupLabel>基础功能</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {baseNavLinks.map((link) => (
                <SidebarMenuItem key={link.to}>
                  <SidebarMenuButton
                    asChild
                    isActive={location.pathname === link.to}
                    tooltip={link.title}
                  >
                    <Link to={link.to}>
                      <link.icon className='size-4' />
                      <span>{link.title}</span>
                    </Link>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>

        {isAdmin && (
          <SidebarGroup>
            <SidebarGroupLabel>管理员功能</SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                {filteredAdminLinks.map((link) => (
                  <SidebarMenuItem key={link.to}>
                    <SidebarMenuButton
                      asChild
                      isActive={location.pathname === link.to}
                      tooltip={link.title}
                    >
                      <Link to={link.to}>
                        <link.icon className='size-4' />
                        <span>{link.title}</span>
                      </Link>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                ))}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        )}
      </SidebarContent>

      <SidebarFooter className='p-4 border-t border-border'>
        <div className='flex items-center justify-between group-data-[collapsible=icon]:flex-col group-data-[collapsible=icon]:gap-4'>
          <UserMenu />
          <div className='flex items-center gap-2 group-data-[collapsible=icon]:hidden'>
            <ColorThemeSelector />
            <ThemeSwitch />
          </div>
        </div>
      </SidebarFooter>
    </Sidebar>
  )
}
