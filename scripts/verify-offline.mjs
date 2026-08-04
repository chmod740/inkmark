import { readFile, readdir } from 'node:fs/promises'
import { join } from 'node:path'

const dist = new URL('../frontend/dist/', import.meta.url)
const html = await readFile(new URL('index.html', dist), 'utf8')
const assetNames = await readdir(new URL('assets/', dist))
const failures = []

if (/<(?:script|link|img)\b[^>]+(?:src|href)=["']https?:\/\//i.test(html)) {
  failures.push('index.html 包含远程脚本、样式或图片资源')
}
if (!/Content-Security-Policy/i.test(html)) {
  failures.push('index.html 缺少 Content-Security-Policy')
}
if (!assetNames.some((name) => name.endsWith('.woff2'))) {
  failures.push('KaTeX 字体没有打包到本地 assets')
}

for (const name of assetNames.filter((item) => item.endsWith('.css'))) {
  const css = await readFile(new URL(`assets/${name}`, dist), 'utf8')
  if (/@import\s+(?:url\()?['"]?https?:\/\//i.test(css) || /url\(['"]?https?:\/\//i.test(css)) {
    failures.push(`${name} 包含远程 CSS 资源`)
  }
}

if (failures.length) {
  failures.forEach((failure) => console.error(`离线检查失败: ${failure}`))
  process.exit(1)
}

console.log(`离线资源检查通过：${assetNames.length} 个资源均由应用本地加载。`)
