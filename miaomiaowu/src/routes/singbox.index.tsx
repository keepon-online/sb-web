import { createFileRoute } from '@tanstack/react-router'
import { SingboxPage } from '@/features/singbox/singbox-page'

export const Route = createFileRoute('/singbox/')({
  component: SingboxPage,
})
