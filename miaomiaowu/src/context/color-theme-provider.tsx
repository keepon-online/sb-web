import { createContext, useContext, useEffect, useState, useMemo } from 'react'
import { getCookie, setCookie } from '@/lib/cookies'

export type ColorTheme = 'slate' | 'indigo' | 'emerald' | 'rose' | 'amber'

const DEFAULT_COLOR_THEME: ColorTheme = 'slate'
const COLOR_THEME_COOKIE_NAME = 'vite-ui-color-theme'
const COLOR_THEME_COOKIE_MAX_AGE = 60 * 60 * 24 * 365 // 1 year

type ColorThemeProviderProps = {
  children: React.ReactNode
  defaultColorTheme?: ColorTheme
  storageKey?: string
}

type ColorThemeProviderState = {
  colorTheme: ColorTheme
  setColorTheme: (colorTheme: ColorTheme) => void
}

const initialState: ColorThemeProviderState = {
  colorTheme: DEFAULT_COLOR_THEME,
  setColorTheme: () => null,
}

const ColorThemeContext = createContext<ColorThemeProviderState>(initialState)

export function ColorThemeProvider({
  children,
  defaultColorTheme = DEFAULT_COLOR_THEME,
  storageKey = COLOR_THEME_COOKIE_NAME,
  ...props
}: ColorThemeProviderProps) {
  const [colorTheme, _setColorTheme] = useState<ColorTheme>(
    () => (getCookie(storageKey) as ColorTheme) || defaultColorTheme
  )

  useEffect(() => {
    const root = window.document.documentElement
    console.log('[ColorTheme] Applying theme:', colorTheme)
    
    // Remove existing color theme classes
    const classesToRemove: string[] = []
    root.classList.forEach((className) => {
      if (className.startsWith('theme-')) {
        classesToRemove.push(className)
      }
    })
    classesToRemove.forEach((className) => root.classList.remove(className))

    // Add the active color theme class
    root.classList.add(`theme-${colorTheme}`)
    console.log('[ColorTheme] Updated document classes:', root.className)
  }, [colorTheme])

  const setColorTheme = (newColorTheme: ColorTheme) => {
    setCookie(storageKey, newColorTheme, COLOR_THEME_COOKIE_MAX_AGE)
    _setColorTheme(newColorTheme)
  }

  const contextValue = useMemo(() => ({
    colorTheme,
    setColorTheme,
  }), [colorTheme])

  return (
    <ColorThemeContext value={contextValue} {...props}>
      {children}
    </ColorThemeContext>
  )
}

export const useColorTheme = () => {
  const context = useContext(ColorThemeContext)

  if (!context) throw new Error('useColorTheme must be used within a ColorThemeProvider')

  return context
}
