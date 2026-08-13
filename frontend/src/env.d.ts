/// <reference types="vite/client" />

declare module 'highlight.js/lib/common'

declare module 'markdown-it-task-lists' {
  import type MarkdownIt from 'markdown-it'
  interface TaskListOptions {
    enabled?: boolean
    label?: boolean
    labelAfter?: boolean
  }
  const taskLists: (markdown: MarkdownIt, options?: TaskListOptions) => void
  export default taskLists
}

declare module 'markdown-it-emoji' {
  import type MarkdownIt from 'markdown-it'
  type EmojiPlugin = (markdown: MarkdownIt) => void
  export const bare: EmojiPlugin
  export const full: EmojiPlugin
  export const light: EmojiPlugin
}

declare module 'markdown-it-footnote' {
  import type MarkdownIt from 'markdown-it'
  const footnote: (markdown: MarkdownIt) => void
  export default footnote
}
