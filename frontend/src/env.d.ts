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

declare module 'markdown-it-abbr' {
  import type MarkdownIt from 'markdown-it'
  const abbreviation: (markdown: MarkdownIt) => void
  export default abbreviation
}

declare module 'markdown-it-deflist' {
  import type MarkdownIt from 'markdown-it'
  const definitionList: (markdown: MarkdownIt) => void
  export default definitionList
}

declare module 'markdown-it-mark' {
  import type MarkdownIt from 'markdown-it'
  const mark: (markdown: MarkdownIt) => void
  export default mark
}

declare module 'markdown-it-sub' {
  import type MarkdownIt from 'markdown-it'
  const subscript: (markdown: MarkdownIt) => void
  export default subscript
}

declare module 'markdown-it-sup' {
  import type MarkdownIt from 'markdown-it'
  const superscript: (markdown: MarkdownIt) => void
  export default superscript
}
