const mathDelimiterPattern = /\$\$(.*?)\$\$/g
const safeHTMLTagSource = String.raw`<\s*\/?\s*(?:br|i|b|sub|sup)\s*\/?\s*>`

const namedSymbols: Record<string, string> = {
  alpha: 'α', beta: 'β', gamma: 'γ', delta: 'δ', epsilon: 'ε', theta: 'θ',
  lambda: 'λ', mu: 'μ', pi: 'π', rho: 'ρ', sigma: 'σ', phi: 'φ', omega: 'ω',
  Gamma: 'Γ', Delta: 'Δ', Theta: 'Θ', Lambda: 'Λ', Pi: 'Π', Sigma: 'Σ', Phi: 'Φ', Omega: 'Ω',
  sum: '∑', prod: '∏', int: '∫', infty: '∞', mid: '|', times: '×', cdot: '·',
  le: '≤', leq: '≤', ge: '≥', geq: '≥', ne: '≠', neq: '≠', approx: '≈', to: '→',
  leftarrow: '←', rightarrow: '→', pm: '±', ldots: '…', dots: '…',
}

const scriptLetters: Record<string, string> = {
  A: '𝒜', B: 'ℬ', C: '𝒞', D: '𝒟', E: 'ℰ', F: 'ℱ', G: '𝒢', H: 'ℋ', I: 'ℐ', J: '𝒥',
  K: '𝒦', L: 'ℒ', M: 'ℳ', N: '𝒩', O: '𝒪', P: '𝒫', Q: '𝒬', R: 'ℛ', S: '𝒮', T: '𝒯',
  U: '𝒰', V: '𝒱', W: '𝒲', X: '𝒳', Y: '𝒴', Z: '𝒵',
}

function escapeHTML(value: string) {
  return value
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;')
}

class InlineTeXHTMLParser {
  private cursor = 0
  private readonly source: string

  constructor(source: string) {
    this.source = source
  }

  parse(stopAtBrace = false): string {
    let output = ''
    while (this.cursor < this.source.length) {
      const character = this.source[this.cursor]
      if (stopAtBrace && character === '}') {
        this.cursor += 1
        break
      }
      if (character === '{') {
        this.cursor += 1
        output += this.parse(true)
        continue
      }
      if (character === '_' || character === '^') {
        this.cursor += 1
        const value = this.parseArgument()
        output += character === '_' ? `<sub>${value}</sub>` : `<sup>${value}</sup>`
        continue
      }
      if (character === '\\') {
        output += this.parseCommand()
        continue
      }
      if (/[A-Za-z]/.test(character)) {
        let end = this.cursor + 1
        while (end < this.source.length && /[A-Za-z]/.test(this.source[end])) end += 1
        output += `<i>${escapeHTML(this.source.slice(this.cursor, end))}</i>`
        this.cursor = end
        continue
      }
      output += escapeHTML(character)
      this.cursor += 1
    }
    return output
  }

  private parseArgument(): string {
    while (this.source[this.cursor] === ' ') this.cursor += 1
    if (this.source[this.cursor] === '{') {
      this.cursor += 1
      return this.parse(true)
    }
    if (this.cursor >= this.source.length) return ''
    if (this.source[this.cursor] === '\\') return this.parseCommand()
    const value = escapeHTML(this.source[this.cursor])
    this.cursor += 1
    return value
  }

  private parseCommand(): string {
    this.cursor += 1
    if (this.cursor >= this.source.length) return '\\'
    if (!/[A-Za-z]/.test(this.source[this.cursor])) {
      const escaped = escapeHTML(this.source[this.cursor])
      this.cursor += 1
      return escaped
    }
    const start = this.cursor
    while (this.cursor < this.source.length && /[A-Za-z]/.test(this.source[this.cursor])) this.cursor += 1
    const command = this.source.slice(start, this.cursor)

    if (command === 'frac') {
      const numerator = this.parseArgument()
      const denominator = this.parseArgument()
      return `<span class='inkmark-mermaid-fraction'><span>${numerator}</span><span>/</span><span>${denominator}</span></span>`
    }
    if (command === 'sqrt') return `<span>√(${this.parseArgument()})</span>`
    if (command === 'mathcal') {
      const argument = this.parseArgument()
      const plainLetter = argument.match(/^<i>([A-Z])<\/i>$/)?.[1]
      return plainLetter ? (scriptLetters[plainLetter] || plainLetter) : `<i>${argument}</i>`
    }
    if (command === 'mathbf') return `<b>${this.parseArgument()}</b>`
    if (command === 'mathrm' || command === 'text') return `<span>${this.parseArgument()}</span>`
    if (command === 'hat' || command === 'bar' || command === 'vec') {
      const accent = command === 'hat' ? '\u0302' : command === 'bar' ? '\u0304' : '\u20d7'
      return `<span class='inkmark-mermaid-accent'>${this.parseArgument()}${accent}</span>`
    }
    const symbol = namedSymbols[command]
    return symbol ? escapeHTML(symbol) : escapeHTML(`\\${command}`)
  }
}

export function renderMermaidMathLabelHTML(label: string) {
  mathDelimiterPattern.lastIndex = 0
  if (!mathDelimiterPattern.test(label)) {
    mathDelimiterPattern.lastIndex = 0
    return undefined
  }
  mathDelimiterPattern.lastIndex = 0
  let output = ''
  let cursor = 0
  for (const match of label.matchAll(mathDelimiterPattern)) {
    const index = match.index || 0
    output += label.slice(cursor, index)
    output += `<span class='inkmark-mermaid-formula'>${new InlineTeXHTMLParser(match[1]).parse()}</span>`
    cursor = index + match[0].length
  }
  output += label.slice(cursor)
  return output
}

export function embedMermaidMathLabelHTML(definition: string) {
  let changed = false
  const output = definition.split('\n').map((line) => {
    // Mermaid comments and init directives share the %% prefix and must stay
    // byte-for-byte intact even when their text happens to contain dollars.
    if (line.trimStart().startsWith('%%')) return line
    const rendered = renderMermaidMathLabelHTML(line)
    changed ||= Boolean(rendered)
    return rendered || line
  }).join('\n')
  return { definition: output, changed }
}

export function normalizeMermaidSafeHTML(definition: string) {
  let hasHTML = false
  const output = definition.split('\n').map((line) => {
    if (line.trimStart().startsWith('%%')) return line
    let normalized = line.replace(
      new RegExp(String.raw`\\(${safeHTMLTagSource})`, 'gi'),
      (_match, tag: string) => {
        hasHTML = true
        return tag
      },
    )
    if (new RegExp(safeHTMLTagSource, 'i').test(normalized)) hasHTML = true
    normalized = normalized.replace(/\|([^|\n]+)\|/g, (match, label: string) => {
      if (!new RegExp(safeHTMLTagSource, 'i').test(label)) return match
      const trimmed = label.trim()
      if (trimmed.startsWith('"') && trimmed.endsWith('"')) return match
      return `|"${label}"|`
    })
    return normalized
  }).join('\n')
  return { definition: output, hasHTML }
}
