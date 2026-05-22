import { createFileRoute, Outlet } from '@tanstack/react-router'
import { Topbar } from '@/components/layout/topbar'

export const Route = createFileRoute('/custom-rules')({
  component: CustomRulesLayout,
})

function CustomRulesLayout() {
  return (
    <div className='bg-background min-h-svh'>
      <Topbar />
      <Outlet />
    </div>
  )
}
