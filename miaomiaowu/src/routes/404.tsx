import { createFileRoute, Link } from '@tanstack/react-router'
import { Home } from 'lucide-react'
import { Button } from '@/components/ui/button'

export const Route = createFileRoute('/404')({
  component: NotFoundPage,
})

function NotFoundPage() {
  return (
    <div className='bg-background flex min-h-svh flex-col items-center justify-center gap-4 px-4 text-center'>
      <h1 className='text-3xl font-semibold tracking-tight'>404 Not Found</h1>
      <Button asChild variant='outline'>
        <Link to='/'>
          <Home className='mr-2 h-4 w-4' />
          返回主页
        </Link>
      </Button>
    </div>
  )
}
