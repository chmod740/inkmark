export const themeIds = [
  'github',
  'clean',
  'wechat',
  'dark',
  'mist',
  'paper',
  'autumn',
  'pine',
  'sakura',
  'ocean',
  'indigo',
  'nord',
  'obsidian',
] as const

export type Theme = typeof themeIds[number]

export type ThemeTranslationKey =
  | 'theme.github'
  | 'theme.clean'
  | 'theme.wechat'
  | 'theme.dark'
  | 'theme.mist'
  | 'theme.paper'
  | 'theme.autumn'
  | 'theme.pine'
  | 'theme.sakura'
  | 'theme.ocean'
  | 'theme.indigo'
  | 'theme.nord'
  | 'theme.obsidian'

export interface ThemePalette {
  background: string
  surface: string
  ink: string
  muted: string
  accent: string
  secondary: string
  border: string
  code: string
}

export interface ThemeDefinition {
  id: Theme
  labelKey: ThemeTranslationKey
  dark: boolean
  palette: ThemePalette
}

export const themeDefinitions: readonly ThemeDefinition[] = [
  {
    id: 'github', labelKey: 'theme.github', dark: false,
    palette: { background: '#ffffff', surface: '#f6f8fa', ink: '#1f2328', muted: '#59636e', accent: '#0969da', secondary: '#8250df', border: '#d0d7de', code: '#24292f' },
  },
  {
    id: 'clean', labelKey: 'theme.clean', dark: false,
    palette: { background: '#ffffff', surface: '#edf0ed', ink: '#293330', muted: '#586762', accent: '#1c6d73', secondary: '#d3a94d', border: '#c3ccc8', code: '#263433' },
  },
  {
    id: 'wechat', labelKey: 'theme.wechat', dark: false,
    palette: { background: '#ffffff', surface: '#f0f8f4', ink: '#2f3437', muted: '#4e5f58', accent: '#1e7569', secondary: '#d6a149', border: '#c7d7d1', code: '#34302d' },
  },
  {
    id: 'dark', labelKey: 'theme.dark', dark: true,
    palette: { background: '#0d1117', surface: '#161b22', ink: '#c9d1d9', muted: '#8b949e', accent: '#58a6ff', secondary: '#d2a8ff', border: '#30363d', code: '#161b22' },
  },
  {
    id: 'mist', labelKey: 'theme.mist', dark: false,
    palette: { background: '#ffffff', surface: '#edf7f5', ink: '#273431', muted: '#71807c', accent: '#3f7d81', secondary: '#d3a94d', border: '#bac4c0', code: '#263433' },
  },
  {
    id: 'paper', labelKey: 'theme.paper', dark: false,
    palette: { background: '#fffdf8', surface: '#f8eee3', ink: '#3b3028', muted: '#86786c', accent: '#9b5c3d', secondary: '#6d8b5b', border: '#d3c8bb', code: '#352f2b' },
  },
  {
    id: 'autumn', labelKey: 'theme.autumn', dark: false,
    palette: { background: '#f7f5f1', surface: '#f0ece6', ink: '#37332d', muted: '#746b61', accent: '#64272d', secondary: '#9b461f', border: '#d8d2c9', code: '#ece9e3' },
  },
  {
    id: 'pine', labelKey: 'theme.pine', dark: false,
    palette: { background: '#fbfdfa', surface: '#e9f4ed', ink: '#26352e', muted: '#6c7d73', accent: '#36715a', secondary: '#b98b43', border: '#b5c3ba', code: '#23342c' },
  },
  {
    id: 'sakura', labelKey: 'theme.sakura', dark: false,
    palette: { background: '#fffdfd', surface: '#faedf1', ink: '#3b3035', muted: '#827078', accent: '#a25469', secondary: '#5a8297', border: '#cfc2c8', code: '#342d31' },
  },
  {
    id: 'ocean', labelKey: 'theme.ocean', dark: false,
    palette: { background: '#fbfeff', surface: '#e8f5f7', ink: '#26383e', muted: '#6b7f85', accent: '#287a93', secondary: '#d69a45', border: '#b4c8ce', code: '#20343b' },
  },
  {
    id: 'indigo', labelKey: 'theme.indigo', dark: true,
    palette: { background: '#181e30', surface: '#222c48', ink: '#e5e9f7', muted: '#98a2bf', accent: '#8c9cff', secondary: '#f0b56a', border: '#38415f', code: '#101522' },
  },
  {
    id: 'nord', labelKey: 'theme.nord', dark: true,
    palette: { background: '#2b313d', surface: '#33424a', ink: '#e5e9f0', muted: '#9aa5b8', accent: '#88c0d0', secondary: '#ebcb8b', border: '#4c566a', code: '#1e2430' },
  },
  {
    id: 'obsidian', labelKey: 'theme.obsidian', dark: true,
    palette: { background: '#101619', surface: '#132624', ink: '#e8f0ee', muted: '#82908d', accent: '#44c7b6', secondary: '#ffb454', border: '#2b3638', code: '#05090b' },
  },
] as const

const themeById = new Map<Theme, ThemeDefinition>(themeDefinitions.map((definition) => [definition.id, definition]))

export function isTheme(value: unknown): value is Theme {
  return typeof value === 'string' && themeById.has(value as Theme)
}

export function normalizeTheme(value: unknown): Theme {
  return isTheme(value) ? value : 'github'
}

export function themeDefinition(theme: Theme): ThemeDefinition {
  return themeById.get(theme) || themeById.get('github')!
}

export function isDarkTheme(theme: Theme): boolean {
  return themeDefinition(theme).dark
}

export function themeBackground(theme: Theme): string {
  return themeDefinition(theme).palette.background
}

export function themePalette(theme: Theme): ThemePalette {
  return themeDefinition(theme).palette
}

export function hexColorChannels(color: string): [number, number, number] {
  const match = /^#([0-9a-f]{6})$/i.exec(color)
  if (!match) return [255, 255, 255]
  return [
    Number.parseInt(match[1].slice(0, 2), 16),
    Number.parseInt(match[1].slice(2, 4), 16),
    Number.parseInt(match[1].slice(4, 6), 16),
  ]
}
