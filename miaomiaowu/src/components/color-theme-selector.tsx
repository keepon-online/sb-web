import { Palette, Check } from 'lucide-react'
import { useColorTheme, type ColorTheme } from '@/context/color-theme-provider'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'

interface ThemeOption {
  value: ColorTheme
  label: string
  colorClass: string // For the dot preview
}

const themeOptions: ThemeOption[] = [
  {
    value: 'slate',
    label: '极简墨黑',
    colorClass: 'bg-slate-600 dark:bg-slate-400',
  },
  {
    value: 'indigo',
    label: '极客靛蓝',
    colorClass: 'bg-indigo-600 dark:bg-indigo-400',
  },
  {
    value: 'emerald',
    label: '生机翡翠',
    colorClass: 'bg-emerald-600 dark:bg-emerald-400',
  },
  {
    value: 'rose',
    label: '时尚红玫',
    colorClass: 'bg-rose-600 dark:bg-rose-400',
  },
  {
    value: 'amber',
    label: '温暖琥珀',
    colorClass: 'bg-amber-600 dark:bg-amber-400',
  },
]

export function ColorThemeSelector() {
  const { colorTheme, setColorTheme } = useColorTheme()

  const currentOption = themeOptions.find((opt) => opt.value === colorTheme) || themeOptions[0]

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="outline"
          size="icon"
          title="选择配色方案"
          aria-label="选择配色方案"
          className="h-9 w-9 relative group"
        >
          <Palette className="size-[18px] transition-transform group-hover:rotate-12 duration-200" />
          <span className={cn(
            "absolute bottom-1 right-1 size-2 rounded-full border border-background shadow-sm transition-all duration-300",
            currentOption.colorClass
          )} />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-[150px] p-1 shadow-xl">
        {themeOptions.map((opt) => (
          <DropdownMenuItem
            key={opt.value}
            onClick={() => setColorTheme(opt.value)}
            className="flex items-center justify-between cursor-pointer py-2 px-2.5 rounded-md transition-colors"
          >
            <div className="flex items-center gap-2.5">
              <span className={cn("size-3 rounded-full shadow-sm transition-transform duration-300", opt.colorClass)} />
              <span className="text-sm font-medium">{opt.label}</span>
            </div>
            {colorTheme === opt.value && (
              <Check className="size-4 text-primary shrink-0" />
            )}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
