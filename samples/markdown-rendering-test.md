---
title: "Markdown 综合渲染测试 / Comprehensive Rendering Test"
category: documentation
offline: true
revision: 2
---

# Markdown 综合渲染测试 / Comprehensive Rendering Test {#rendering-test .bilingual}

这是一份用于验证常见 CommonMark、GitHub 风格扩展、表情、脚注、KaTeX 与多种图表的内置双语文档。

This built-in bilingual document exercises common CommonMark syntax, GitHub-style extensions, emoji, footnotes, KaTeX, and multiple diagram formats.

[返回欢迎页 / Back to Welcome](#inkmark-welcome)

[TOC]

## 基础文本 / Basic Text

普通文本支持 *斜体 / italic*、**粗体 / bold**、***粗斜体 / bold italic***、~~删除线 / strikethrough~~、==高亮 / highlight==、`inline code`、上标 x^2^ 与下标 H~2~O。

连续两个空格产生硬换行。  
This line follows a hard line break.

自动链接 / Autolinks：https://commonmark.org/ and <support@example.com>.

参考链接 / Reference links：[CommonMark 规范][commonmark] and [Mermaid documentation][mermaid].

[commonmark]: https://spec.commonmark.org/ "CommonMark Spec"
[mermaid]: https://mermaid.js.org/ "Mermaid Documentation"

### 通用方言扩展 / General Dialect Extensions {#dialect-extensions .demo}

HTML 与 CSS 缩写会显示说明；HTML and CSS abbreviations expose their definitions.

Markdown 方言
: 支持定义列表、缩写、高亮、上下标、目录、标题属性、Wiki 链接、Front Matter 与本地引用文献。

Markdown dialects
: Definition lists, abbreviations, mark, sub/sup, TOC, heading attributes, wiki links, front matter, and local citations.

Wiki 链接在纯离线预览中显示为待解析文本：[[项目首页|Project Home]]；它不会擅自读取文件或联网。

引用示例 / Citation example：安全渲染必须保持离线[@inkmark-rendering]，扩展语法遵循明确边界[@markdown-dialects]。

*[HTML]: HyperText Markup Language
*[CSS]: Cascading Style Sheets
[@inkmark-rendering]: InkMark 团队，《安全的离线 Markdown 渲染》，2026。
[@markdown-dialects]: InkMark Team, “Bounded Markdown Dialect Extensions,” 2026.

---

## 标题层级 / Heading Levels

### 三级标题 / Heading 3

#### 四级标题 / Heading 4

##### 五级标题 / Heading 5

###### 六级标题 / Heading 6

---

## GitHub 风格提示块 / GitHub-style Alerts

> [!NOTE]
> 说明类型，用于补充背景信息。A note adds useful context.

> [!TIP]
> 提示可以包含 **强调**、`code` 和 [link](https://example.com).

> [!IMPORTANT]
> 重要内容需要关注。Important information needs attention.

> [!WARNING]
> 警告表示潜在风险。A warning indicates potential risk.

> [!CAUTION]
> 注意可能造成损失的操作。Use caution before a destructive action.

## 表情、提及与脚注 / Emoji, Mentions, and Footnotes

表情别名 / Emoji aliases: :smile: :+1: :rocket: :tada:

普通文本中的提及会以标签显示：@Vanessa 与 @inkmark-team；代码 `@not-a-mention` 和链接 [@linked-user](https://example.com/users/linked-user) 保持原样。

Plain-text mentions are shown as labels: @Vanessa and @inkmark-team; code `@not-a-mention` and the link [@linked-user](https://example.com/users/linked-user) remain unchanged.

脚注支持简短说明[^short-note]，也支持包含多个段落的长说明[^long-note]。

Footnotes support a short note[^short-note] and a longer, multi-paragraph note[^long-note].

[^short-note]: 这是简短的中英双语脚注。This is a short bilingual footnote.
[^long-note]: 第一段保留 **粗体 / bold**、`inline code` 与 [链接 / link](https://example.com).

    第二段验证缩进续写。The second paragraph verifies an indented continuation.

## 兼容提示块 / Legacy Callouts

> NOTE: 旧式说明支持 **粗体**、`inline code` 和 [外部链接](https://example.com)，这些行内节点不会在识别标题时丢失。

> Tip: A legacy tip keeps **strong text**, `code`, and an [external link](https://example.com).
>
> 第二段仍属于同一个提示块。The second paragraph remains inside the same callout.

## 引用与嵌套引用 / Blockquotes

> 第一层引用 / First-level quote
>
> > 第二层引用包含列表 / A nested quote contains a list:
> > 1. 引用中的有序项 / Ordered item
> > 2. 第二项 / Second item
>
> 返回第一层 / Back to the first level.

## 列表与任务 / Lists and Tasks

1. 第一项 / First item
   1. 嵌套有序项 / Nested ordered item
   2. 另一个嵌套项 / Another nested item
2. 第二项 / Second item
   - 无序子项 A / Unordered child A
   - 无序子项 B / Unordered child B
     - 第三级列表 / Third-level list
3. 第三项 / Third item

- [x] 基础语法 / Basic syntax
- [x] 表格渲染 / Table rendering
- [ ] 待完成任务 / Pending task
- [ ] 需要人工确认 / Manual confirmation

## 复杂表格 / Complex Table

| 左对齐功能 / Left | 居中状态 / Center | 右对齐数值 / Right | 特殊内容 / Special |
| :--- | :---: | ---: | --- |
| 普通文本 / Text | 通过 / Pass | 1,024 | 单元格包含 `code` |
| **粗体** 与 *斜体* | Pass | 98.75% | [外部链接 / External link](https://example.com) |
| ~~旧值~~ 新值 | Updated | -42 | 转义竖线 / Escaped pipe A \| B |
| 行内公式 $a^2+b^2=c^2$ | 通过 / Pass | 3.14159 | 中文、English、123 |

## 数学公式 / Mathematics

行内公式 / Inline formula: $E=mc^2$.

括号公式 / Parenthesized formula: \(e^{i\pi}+1=0\).

块级公式 / Display formula:

$$
\frac{\partial}{\partial x}\left(\int_{-\infty}^{x} e^{-t^2}\,dt\right)=e^{-x^2}
$$

\[
\begin{aligned}
A &= \pi r^2 \\
V &= \frac{4}{3}\pi r^3 \\
\sum_{k=1}^{n} k &= \frac{n(n+1)}{2}
\end{aligned}
\]

矩阵与集合 / Matrix and set:

$$
\mathbf{A}=\begin{pmatrix}1&2\\3&4\end{pmatrix},\qquad
\mathbb{R}=\{x\mid -\infty < x < \infty\}
$$

行内代码中的美元符号不能变成公式 / A dollar sign inside code must remain code: `const price = "$19.99";`.

## 代码高亮 / Syntax Highlighting

~~~javascript
class MarkdownPreview {
    constructor(theme = 'github') {
        this.theme = theme;
    }

    render(source) {
        return { source, theme: this.theme, safe: true };
    }
}
~~~

~~~python
from dataclasses import dataclass

@dataclass
class Result:
    tables: int
    formulas: int
    diagrams: int

print(Result(tables=1, formulas=5, diagrams=10))
~~~

~~~go
package main

import "fmt"

func main() {
    fmt.Println("本地 Markdown / Local Markdown")
}
~~~

~~~rust
fn main() {
    println!("安全渲染 / Safe rendering");
}
~~~

~~~json
{
  "name": "inkmark-render-test",
  "features": ["gfm", "katex", "mermaid"],
  "enabled": true
}
~~~

~~~bash
set -euo pipefail
printf 'Code fence using tildes: %s\n' 'OK'
~~~

## 扩展图表 / Extended Diagrams

### 缩进列表思维导图 / Indented-list Mind Map

~~~mindmap
- 教程 / Tutorials
- 语法指导 / Syntax Guide
  - 普通内容 / Plain Content
  - 提及用户 @Vanessa / Mention a User
    - 表情符号 :smile: / Emoji
      - 四级节点 / Level 4
        - 五级节点 / Level 5
          - 六级节点 / Level 6
- 快捷键 / Shortcuts
  - 保存 / Save
  - 预览 / Preview
~~~

### ECharts 折线图 / ECharts Line Chart

~~~echarts
{
  "title": { "text": "月度预览 / Monthly Previews" },
  "tooltip": { "trigger": "axis" },
  "legend": { "data": ["文档 / Documents", "导出 / Exports"] },
  "xAxis": { "type": "category", "data": ["一月 / Jan", "二月 / Feb", "三月 / Mar", "四月 / Apr"] },
  "yAxis": { "type": "value" },
  "series": [
    { "name": "文档 / Documents", "type": "line", "smooth": true, "data": [42, 65, 88, 103] },
    { "name": "导出 / Exports", "type": "line", "smooth": true, "data": [18, 34, 55, 79] }
  ]
}
~~~

### ABC 乐谱 / ABC Music Notation

~~~abc
X:1
T:InkMark Test Tune
C:Traditional
M:4/4
L:1/8
K:C
|: C2 E2 G2 c2 | B2 G2 A4 | F2 A2 G2 E2 | D2 C2 C4 :|
~~~

### Graphviz 关系图 / Graphviz Relationship Diagram

~~~graphviz
digraph InkMark {
  rankdir=LR;
  node [shape=box, style="rounded"];
  Markdown -> Parse [label="解析 / Parse"];
  Parse -> Preview [label="渲染 / Render"];
  Preview -> Export [label="导出 / Export"];
}
~~~

## 流程图 / Flowchart

~~~mermaid
flowchart TD
    START([开始 / Start]) --> INPUT[读取 Markdown / Read Markdown]
    INPUT --> TYPE{识别内容 / Detect content}
    TYPE -->|GFM| GFM[表格与任务 / Tables and tasks]
    TYPE -->|Math| MATH[KaTeX 公式 / Formula]
    TYPE -->|Diagram| CHART[Mermaid 图表 / Diagram]
    GFM --> SAFE[安全过滤 / Sanitize]
    MATH --> SAFE
    CHART --> SAFE
    SAFE --> DONE([完成 / Done])
~~~

## 时序图 / Sequence Diagram

~~~mermaid
sequenceDiagram
    autonumber
    actor User
    participant Editor
    participant Renderer
    participant LocalFile as Local File
    User->>Editor: 打开文档 / Open document
    Editor->>LocalFile: 读取 UTF-8 / Read UTF-8
    LocalFile-->>Editor: Markdown source
    Editor->>Renderer: GFM + KaTeX + Mermaid
    Renderer-->>User: 实时预览 / Live preview
~~~

## 类图 / Class Diagram

~~~mermaid
classDiagram
    class MarkdownRenderer {
        +render(source)
        +sanitize(html)
        +decorate(target)
    }
    class PreviewController {
        +applyTheme(theme)
        +syncScroll()
    }
    class MermaidAdapter {
        +renderDiagrams(target)
    }
    MarkdownRenderer --> MermaidAdapter
    PreviewController --> MarkdownRenderer
~~~

## 状态图 / State Diagram

~~~mermaid
stateDiagram-v2
    [*] --> Loading
    Loading --> Rendering: dependencies ready
    Loading --> Failed: timeout
    Rendering --> Ready: success
    Rendering --> Failed: syntax error
    Ready --> Rendering: source changed
    Failed --> Loading: retry
~~~

## ER 图 / ER Diagram

~~~mermaid
erDiagram
    DOCUMENT ||--o{ EXPORT : produces
    DOCUMENT ||--o{ REVISION : contains
    DOCUMENT {
        string path
        string encoding
    }
    EXPORT {
        string format
        datetime created_at
    }
    REVISION {
        int sequence
        string content
    }
~~~

## 甘特图 / Gantt Chart

~~~mermaid
gantt
    title 本地文档工作流 / Local Document Workflow
    dateFormat YYYY-MM-DD
    axisFormat %m-%d
    section Write
    编辑内容 / Edit content       :done, edit, 2026-08-01, 2d
    实时预览 / Live preview       :done, preview, after edit, 1d
    section Verify
    检查图表 / Verify diagrams    :active, verify, 2026-08-04, 2d
    导出文档 / Export document    :export, after verify, 1d
~~~

## 饼图 / Pie Chart

~~~mermaid
pie showData title Markdown Rendering Coverage
    "CommonMark / GFM" : 35
    "KaTeX" : 20
    "Mermaid" : 25
    "Themes / 主题" : 20
~~~

## 象限图 / Quadrant Chart

~~~mermaid
quadrantChart
    title Feature Priority Matrix
    x-axis Low Effort --> High Effort
    y-axis Low Value --> High Value
    quadrant-1 Strategic
    quadrant-2 Quick Wins
    quadrant-3 Backlog
    quadrant-4 Reconsider
    Tables: [0.20, 0.80]
    Math: [0.35, 0.75]
    Flowcharts: [0.55, 0.85]
    Themes: [0.70, 0.65]
    Experimental: [0.85, 0.25]
~~~

## XY 柱线组合图 / XY Chart

~~~mermaid
xychart
    title "月度预览次数 / Monthly Previews"
    x-axis ["Jan", "Feb", "Mar", "Apr", "May", "Jun"]
    y-axis "Previews" 0 --> 160
    bar [42, 65, 88, 103, 126, 148]
    line [35, 58, 79, 98, 120, 152]
~~~

## 思维导图 / Mind Map

~~~mermaid
mindmap
  root((Markdown))
    基础 / Basics
      标题 / Headings
      列表 / Lists
      表格 / Tables
    数学 / Math
      行内 / Inline
      块级 / Display
    图表 / Diagrams
      流程 / Flow
      时序 / Sequence
      统计 / Charts
    展示 / Presentation
      GitHub
      Clean
      WeChat
      Dark
~~~

## 折叠内容与安全 HTML / Details and Safe HTML

<details>
<summary>展开查看 / Expand details</summary>

折叠区域包含 **Markdown 文本**、`code` 和列表 / The details block contains Markdown, code, and a list:

- 第一项 / First item
- 第二项 / Second item

</details>

以下脚本标签必须被过滤且绝不执行 / The following script element must be removed and never execute:

<script>window.markdownUnsafeScriptExecuted = true;</script>

---

## 最终检查 / Final Checklist

- [x] CommonMark 基础语法 / Basic syntax
- [x] GitHub 风格扩展 / GitHub-style extensions
- [x] 表情、提及与脚注 / Emoji, mentions, and footnotes
- [x] 兼容提示块 / Legacy callouts
- [x] KaTeX 数学公式 / Mathematics
- [x] Mermaid 十类图表 / Ten diagram types
- [x] 思维导图、ECharts、ABC 与 Graphviz / Mind map, ECharts, ABC, and Graphviz
- [x] 中文与 English 混排 / Bilingual typography
- [x] 多主题与暗色模式 / Themes and dark mode
- [x] 宽表格滚动 / Wide table scrolling
- [x] 危险 HTML 过滤 / Unsafe HTML sanitization
