import { useLocation } from '@tanstack/react-router'
import { SidebarProvider, SidebarInset, SidebarTrigger } from '@/components/ui/sidebar'
import { AppSidebar } from './app-sidebar'
import { UserMenu } from './user-menu'

export function MainLayout({ children }: { children: React.ReactNode }) {
  const location = useLocation()
  const isLoginPage = location.pathname === '/login'

  if (isLoginPage) {
    return <>{children}</>
  }

  return (
    <SidebarProvider>
      <AppSidebar />
      <SidebarInset>
        <header className='flex h-16 shrink-0 items-center justify-between border-b px-4 md:hidden'>
          <div className='flex items-center gap-2'>
            <SidebarTrigger />
            <div className='flex items-center gap-2 font-bold text-primary'>
              <img src='/images/logo.webp' alt='Logo' className='size-6 rounded-md' />
              <span>妙妙屋</span>
            </div>
          </div>
          <UserMenu />
        </header>
        <div className='flex flex-1 flex-col gap-4 p-4 pt-0 md:pt-4'>
          {children}
        </div>
      </SidebarInset>
    </SidebarProvider>
  )
}
