export type ExportFormat = 'pdf' | 'html' | 'png' | 'txt' | 'doc'
export type ExportTheme = 'github' | 'clean' | 'wechat' | 'dark'

export const externalExportStyles = {
  katex: 'https://cdn.jsdelivr.net/npm/katex@0.18.1/dist/katex.min.css',
  highlightLight: 'https://cdn.jsdelivr.net/npm/highlight.js@11.11.1/styles/github.min.css',
  highlightDark: 'https://cdn.jsdelivr.net/npm/highlight.js@11.11.1/styles/github-dark.min.css',
} as const

interface StandaloneHTMLInput {
  title: string
  theme: ExportTheme
  articleHTML: string
  embeddedStyles: string
	  language?: 'zh-CN' | 'en'
  wordCompatible?: boolean
}

const exportOverrides = `
html, body {
  width: auto !important;
  height: auto !important;
  min-height: 100% !important;
  margin: 0 !important;
  overflow: visible !important;
}
body.export-document {
  padding: 32px;
  background: #eef0ed;
}
body.export-document.theme-dark {
  background: #0d1117;
}
.export-document .markdown-body {
  min-height: 0;
  margin-right: auto;
  margin-left: auto;
  overflow: visible;
  box-shadow: 0 2px 12px rgba(15, 23, 42, .12);
}
.export-document.theme-github .markdown-body,
.export-document.theme-dark .markdown-body {
  max-width: 920px;
}
.export-document a {
  overflow-wrap: anywhere;
}
@media print {
  @page { size: A4; margin: 14mm; }
  body.export-document { padding: 0; background: transparent; }
  .export-document .markdown-body {
    max-width: none;
    min-height: 0;
    padding: 0;
    box-shadow: none;
  }
  .export-document pre,
  .export-document blockquote,
  .export-document table,
  .export-document svg,
  .export-document img { break-inside: avoid; }
}
`

export function buildStandaloneHTML(input: StandaloneHTMLInput): string {
	  const title = escapeHTML(input.title || (input.language === 'zh-CN' ? '未命名' : 'Untitled'))
  const articleHTML = normalizeProtocolRelativeResources(input.articleHTML)
  const safeStyles = input.embeddedStyles.replace(/<\/style/gi, '<\\/style')
  const highlightStyle = input.theme === 'dark'
    ? externalExportStyles.highlightDark
    : externalExportStyles.highlightLight
  const officeNamespaces = input.wordCompatible
    ? ' xmlns:o="urn:schemas-microsoft-com:office:office" xmlns:w="urn:schemas-microsoft-com:office:word" xmlns="http://www.w3.org/TR/REC-html40"'
    : ''
  const officeMetadata = input.wordCompatible
	    ? '<meta name="ProgId" content="Word.Document" />\n    <meta name="Generator" content="InkMark Markdown" />'
	    : '<meta name="generator" content="InkMark Markdown" />'
  const bom = input.wordCompatible ? '\uFEFF' : ''

  return `${bom}<!doctype html>
	<html${officeNamespaces} lang="${input.language || 'en'}" data-color-scheme="${input.theme === 'dark' ? 'dark' : 'light'}">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline' https:; font-src data: https:; img-src data: blob: file: http: https:; media-src data: blob: file: http: https:; script-src 'none'; connect-src 'none'; object-src 'none'; frame-src 'none'; base-uri 'none'; form-action 'none'" />
    ${officeMetadata}
    <title>${title}</title>
    <style>
${safeStyles}
${exportOverrides}
    </style>
    <link rel="stylesheet" href="${externalExportStyles.katex}" />
    <link rel="stylesheet" href="${highlightStyle}" />
  </head>
  <body class="export-document theme-${input.theme}">
    <article class="markdown-body">${articleHTML}</article>
  </body>
</html>
`
}

export function normalizeProtocolRelativeResources(value: string): string {
  // A protocol-relative URL inherits file: when a standalone export is opened
  // from disk. Normalising resource attributes to HTTPS keeps them portable on
  // macOS and Windows. Limit rewriting to start tags so text and code examples
  // that happen to contain src="//..." remain byte-for-byte unchanged.
  return value.replace(/<[a-z][^>]*>/gi, (tag) => {
    const withSingleURLs = tag.replace(
      /\b(src|href|poster)\s*=\s*(['"])\/\/([^'"<>]*)\2/gi,
      (_match, attribute: string, quote: string, resource: string) =>
        `${attribute}=${quote}https://${resource}${quote}`,
    )
    return withSingleURLs.replace(
      /\bsrcset\s*=\s*(['"])([^'"]*)\1/gi,
      (_match, quote: string, candidates: string) => {
        const normalized = candidates.replace(/(^|,\s*)\/\/([^\s,]+)/g, '$1https://$2')
        return `srcset=${quote}${normalized}${quote}`
      },
    )
  })
}

export function collectEmbeddedStyles(documentObject: Document = document): string {
  const rules: string[] = []
  for (const sheet of Array.from(documentObject.styleSheets)) {
    try {
      for (const rule of Array.from(sheet.cssRules)) {
        // KaTeX's bundled font URLs point into the app's private /assets tree.
        // The standalone document gets portable font-face rules from the
        // external KaTeX stylesheet declared after the embedded styles.
        if (/^@font-face\b/i.test(rule.cssText.trim())) continue
        rules.push(rule.cssText)
      }
    } catch {
      // A browser may expose a stylesheet without allowing cssRules access.
      // The application styles are local, so this is a defensive fallback.
    }
  }
  return rules.join('\n')
}

export function utf8ToBase64(value: string): string {
  return bytesToBase64(new TextEncoder().encode(value))
}

export function bytesToBase64(bytes: Uint8Array): string {
  const chunkSize = 0x8000
  let binary = ''
  for (let offset = 0; offset < bytes.length; offset += chunkSize) {
    binary += String.fromCharCode(...bytes.subarray(offset, offset + chunkSize))
  }
  return btoa(binary)
}

function escapeHTML(value: string): string {
  return value
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;')
}
