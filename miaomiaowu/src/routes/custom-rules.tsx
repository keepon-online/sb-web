import { createFileRoute, Outlet } from '@tanstack/react-router'

export const Route = createFileRoute('/custom-rules')({
  component: CustomRulesLayout,
})

function CustomRulesLayout() {
  return (
    <div className='bg-background min-h-svh'>
      <Outlet />
    </div>
  )
}
