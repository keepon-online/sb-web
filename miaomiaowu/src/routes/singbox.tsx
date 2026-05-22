// @ts-nocheck
import {
  createFileRoute,
  Outlet,
  redirect,
  useLocation,
} from '@tanstack/react-router'
import { useAuthStore } from '@/stores/auth-store'
import { SingboxPage } from '@/features/singbox/singbox-page'

export const Route = createFileRoute('/singbox')({
  beforeLoad: () => {
    const token = useAuthStore.getState().auth.accessToken
    if (!token) {
      throw redirect({ to: '/' })
    }
  },
  component: SingboxShell,
})

function SingboxShell() {
  const pathname = useLocation({ select: (location) => location.pathname })
  return (
    <div className='bg-background min-h-svh'>
      <main className='pt-0'>
        {pathname === '/singbox' ? <SingboxPage /> : <Outlet />}
      </main>
    </div>
  )
}
