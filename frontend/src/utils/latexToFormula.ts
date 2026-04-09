/**
 * Converts a LaTeX expression to formula text that the backend parser can process.
 * Handles the LaTeX subset produced by formulaTextToLatex, plus common hand-written
 * LaTeX equivalents (\times, \geq, etc.).
 */

/**
 * Extract the content between matching braces starting at `pos` (which must point to `{`).
 * Returns the inner content string and the index after the closing `}`.
 */
function extractBraced(s: string, pos: number): { content: string; end: number } {
  if (pos >= s.length || s[pos] !== '{') return { content: '', end: pos }
  let depth = 1
  let content = ''
  let i = pos + 1
  while (i < s.length) {
    const c = s[i]
    // Escaped brace: \{ or \}
    if (c === '\\' && i + 1 < s.length && (s[i + 1] === '{' || s[i + 1] === '}')) {
      content += c + s[i + 1]
      i += 2
      continue
    }
    if (c === '{') {
      depth++
      content += c
      i++
      continue
    }
    if (c === '}') {
      depth--
      if (depth === 0) return { content, end: i + 1 }
      content += c
      i++
      continue
    }
    content += c
    i++
  }
  return { content, end: i }
}

/**
 * Find the matching \right<closeDelim> for a \left<openDelim> that was already consumed.
 * `pos` points to the character right after \left<openDelim>.
 * Returns inner content and the index after \right<closeDelim>.
 */
function extractLeftRight(s: string, pos: number, _closeDelim: string): { content: string; end: number } {
  let depth = 1
  let content = ''
  let i = pos
  while (i < s.length) {
    if (s.slice(i, i + 5) === '\\left') {
      depth++
      content += '\\left'
      i += 5
      if (i < s.length) { content += s[i]; i++ }
      continue
    }
    if (s.slice(i, i + 6) === '\\right') {
      depth--
      if (depth === 0) {
        // Consumed \right + closeDelim (7 chars total)
        return { content, end: i + 7 }
      }
      content += '\\right'
      i += 6
      if (i < s.length) { content += s[i]; i++ }
      continue
    }
    content += s[i]
    i++
  }
  return { content, end: i }
}

/**
 * Split a comma-separated argument list at the top level (respecting brace/left-right nesting).
 */
function splitTopLevelCommas(s: string): string[] {
  const parts: string[] = []
  let depth = 0
  let current = ''
  let i = 0
  while (i < s.length) {
    if (s.slice(i, i + 5) === '\\left') {
      depth++
      current += '\\left'
      i += 5
      if (i < s.length) { current += s[i]; i++ }
      continue
    }
    if (s.slice(i, i + 6) === '\\right') {
      depth--
      current += '\\right'
      i += 6
      if (i < s.length) { current += s[i]; i++ }
      continue
    }
    const c = s[i]
    if (c === '{') depth++
    else if (c === '}') depth--
    if (c === ',' && depth === 0) {
      parts.push(current.trim())
      current = ''
      i++
      continue
    }
    current += c
    i++
  }
  if (current.trim()) parts.push(current.trim())
  return parts
}

/**
 * Find the matching \end{cases} for a \begin{cases} whose body starts at `pos`.
 * Returns the index of the `\end{cases}` at depth 0, or -1 if not found.
 */
function findMatchingEndCases(s: string, pos: number): number {
  let depth = 1
  let i = pos
  while (i < s.length) {
    if (s.slice(i, i + 12) === '\\begin{cases') {
      depth++
      i += 12
      continue
    }
    if (s.slice(i, i + 11) === '\\end{cases}') {
      depth--
      if (depth === 0) return i
      i += 11
      continue
    }
    i++
  }
  return -1
}

/**
 * Find the first top-level `\\` (double backslash) in a LaTeX string, skipping
 * over any nested \begin{cases}...\end{cases} blocks.
 * Returns the index of the first `\\` at nesting depth 0, or -1 if not found.
 */
function findTopLevelDoublBackslash(s: string): number {
  let depth = 0
  let i = 0
  while (i < s.length) {
    if (s.slice(i, i + 12) === '\\begin{cases') {
      depth++
      i += 12
      continue
    }
    if (s.slice(i, i + 11) === '\\end{cases}') {
      depth--
      i += 11
      continue
    }
    if (depth === 0 && s[i] === '\\' && s[i + 1] === '\\') {
      return i
    }
    i++
  }
  return -1
}

/**
 * Find the first top-level `& \text{otherwise}` marker in a LaTeX string,
 * skipping over any nested \begin{cases}...\end{cases} blocks.
 * Returns the index of the marker start at nesting depth 0, or -1 if not found.
 */
function findTopLevelOtherwiseMarker(s: string): number {
  const marker = '& \\text{otherwise}'
  let depth = 0
  let i = 0
  while (i < s.length) {
    if (s.slice(i, i + 12) === '\\begin{cases') {
      depth++
      i += 12
      continue
    }
    if (s.slice(i, i + 11) === '\\end{cases}') {
      depth--
      i += 11
      continue
    }
    if (depth === 0 && s.slice(i, i + marker.length) === marker) {
      return i
    }
    i++
  }
  return -1
}

/**
 * Transform a `cases` environment body into an if/then/else expression.
 * Expected format (from formulaTextToLatex):
 *   thenExpr, & \text{if } condExpr \\
 *   elseExpr, & \text{otherwise}
 *
 * Handles nested conditionals (nested \begin{cases}) in the else branch.
 */
function transformCases(body: string): string {
  // Find the first top-level \\ separator (not inside a nested cases block)
  const sepIdx = findTopLevelDoublBackslash(body)
  if (sepIdx === -1) return transformLatex(body)

  const thenLine = body.slice(0, sepIdx).trim()
  const elseLine = body.slice(sepIdx + 2).trim()

  // thenLine: "thenExpr, & \text{if } condExpr"
  const ifMarker = '& \\text{if }'
  const ifIdx = thenLine.indexOf(ifMarker)
  if (ifIdx === -1) return transformLatex(body)

  const thenLatex = thenLine.slice(0, ifIdx).replace(/,\s*$/, '').trim()
  const condLatex = thenLine.slice(ifIdx + ifMarker.length).trim()

  // elseLine: "elseExpr, & \text{otherwise}"
  // Use a nesting-aware search so nested \begin{cases} blocks are skipped.
  const otherwiseIdx = findTopLevelOtherwiseMarker(elseLine)
  const elseLatex = otherwiseIdx !== -1
    ? elseLine.slice(0, otherwiseIdx).replace(/,\s*$/, '').trim()
    : elseLine.replace(/,?\s*&.*$/, '').trim()

  const cond = transformLatex(condLatex)
  const thenExpr = transformLatex(thenLatex)
  const elseExpr = transformLatex(elseLatex)

  return `if ${cond} then ${thenExpr} else ${elseExpr}`
}

function skipSpaces(s: string, i: number): number {
  while (i < s.length && (s[i] === ' ' || s[i] === '\t')) i++
  return i
}

/**
 * Recursively transform a LaTeX string to formula text.
 */
function transformLatex(latex: string): string {
  let result = ''
  let i = 0
  const s = latex

  while (i < s.length) {
    const c = s[i]

    // Whitespace
    if (c === ' ' || c === '\t' || c === '\r' || c === '\n') {
      result += ' '
      i++
      continue
    }

    // Alignment marker — skip
    if (c === '&') {
      i++
      continue
    }

    // Double backslash (line separator in aligned/cases)
    if (c === '\\' && s[i + 1] === '\\') {
      result += '\n'
      i += 2
      continue
    }

    // Backslash command
    if (c === '\\') {
      i++
      let cmd = ''
      while (i < s.length && /[a-zA-Z]/.test(s[i])) {
        cmd += s[i++]
      }

      switch (cmd) {
        case 'frac': {
          i = skipSpaces(s, i)
          const num = extractBraced(s, i)
          i = num.end
          i = skipSpaces(s, i)
          const den = extractBraced(s, i)
          i = den.end
          result += `(${transformLatex(num.content)}) / (${transformLatex(den.content)})`
          break
        }

        case 'sqrt': {
          i = skipSpaces(s, i)
          // Optional [...] argument means nth-root (e.g. \sqrt[3]{x} = cube root)
          // The formula DSL only has sqrt() (square root), so we cannot convert this.
          if (s[i] === '[') {
            let nthRoot = ''
            i++ // skip '['
            while (i < s.length && s[i] !== ']') { nthRoot += s[i++] }
            if (i < s.length) i++ // skip ']'
            throw new Error(
              `\\sqrt[${nthRoot}]{...} (nth-root) is not supported. The formula DSL only has sqrt() for square roots.`
            )
          }
          i = skipSpaces(s, i)
          const arg = extractBraced(s, i)
          i = arg.end
          result += `sqrt(${transformLatex(arg.content)})`
          break
        }

        case 'mathrm':
        case 'mathbf':
        case 'mathit':
        case 'mathsf': {
          i = skipSpaces(s, i)
          const arg = extractBraced(s, i)
          i = arg.end
          // Unescape \_ → _ (formulaTextToLatex escapes underscores in identifiers)
          result += arg.content.replace(/\\_/g, '_')
          break
        }

        case 'text': {
          i = skipSpaces(s, i)
          const arg = extractBraced(s, i)
          i = arg.end
          const t = arg.content.trim()
          if (t === 'if' || t === 'if ') { result += '__IFKW__'; break }
          if (t === 'otherwise') { result += '__OTHERWISE__'; break }
          result += `"${t}"`
          break
        }

        case 'sum':
        case 'prod':
        case 'min':
        case 'max': {
          // \sum_{iter=start}^{end} \operatorname{body}\!\left(iter\right)
          // or \operatorname{avg}_{...}^{...} / \operatorname{count}_{...}^{...}
          const aggMap: Record<string, string> = { sum: 'sum', prod: 'product', min: 'min', max: 'max' }
          const agg = aggMap[cmd] ?? cmd
          i = skipSpaces(s, i)
          // Parse subscript _{iter=start}
          let iter = 't', startExpr = '1', endExpr = 'n'
          if (s[i] === '_') {
            i++ // skip _
            i = skipSpaces(s, i)
            const sub = extractBraced(s, i)
            i = sub.end
            const eqIdx = sub.content.indexOf('=')
            if (eqIdx !== -1) {
              iter = transformLatex(sub.content.slice(0, eqIdx)).trim()
              startExpr = transformLatex(sub.content.slice(eqIdx + 1)).trim()
            }
          }
          i = skipSpaces(s, i)
          // Parse superscript ^{end}
          if (s[i] === '^') {
            i++ // skip ^
            i = skipSpaces(s, i)
            const sup = extractBraced(s, i)
            i = sup.end
            endExpr = transformLatex(sup.content).trim()
          }
          i = skipSpaces(s, i)
          // Parse body: \operatorname{name}\!\left(iter\right) or just a token
          let bodyId = ''
          if (s.slice(i, i + 14) === '\\operatorname{' || s.slice(i, i + 15) === '\\operatorname {') {
            // Skip \operatorname
            i += 14
            if (s[i - 1] === ' ') i++ // handle space before {
            // Find the { after \operatorname
            const braceStart = s.indexOf('{', i - 2)
            if (braceStart !== -1) {
              const nameContent = extractBraced(s, braceStart)
              i = nameContent.end
              bodyId = nameContent.content.replace(/\\_/g, '_')
            }
            // Skip optional \! and \left(...\right)
            i = skipSpaces(s, i)
            if (s.slice(i, i + 2) === '\\!') i += 2
            i = skipSpaces(s, i)
            if (s.slice(i, i + 6) === '\\left(') {
              i += 6
              const { end } = extractLeftRight(s, i, ')')
              i = end
            }
          } else {
            // Just grab the next identifier-like token
            while (i < s.length && /[a-zA-Z0-9_]/.test(s[i])) { bodyId += s[i++] }
          }
          result += `${agg}_loop("${bodyId}", ${iter}, ${startExpr}, ${endExpr})`
          break
        }

        case 'operatorname': {
          i = skipSpaces(s, i)
          const name = extractBraced(s, i)
          i = name.end
          i = skipSpaces(s, i)
          // Check if this is a loop aggregation: \operatorname{avg}_{iter=start}^{end} body
          const loopAggs: Record<string, string> = { count: 'count', avg: 'avg', last: 'last' }
          if (loopAggs[name.content] && s[i] === '_') {
            const agg = loopAggs[name.content]
            let iter = 't', startExpr = '1', endExpr = 'n'
            i++ // skip _
            i = skipSpaces(s, i)
            const sub = extractBraced(s, i)
            i = sub.end
            const eqIdx = sub.content.indexOf('=')
            if (eqIdx !== -1) {
              iter = transformLatex(sub.content.slice(0, eqIdx)).trim()
              startExpr = transformLatex(sub.content.slice(eqIdx + 1)).trim()
            }
            i = skipSpaces(s, i)
            if (s[i] === '^') {
              i++
              i = skipSpaces(s, i)
              const sup = extractBraced(s, i)
              i = sup.end
              endExpr = transformLatex(sup.content).trim()
            }
            i = skipSpaces(s, i)
            let bodyId = ''
            if (s.slice(i, i + 14) === '\\operatorname{' || s.slice(i, i + 15) === '\\operatorname {') {
              i += 14
              if (s[i - 1] === ' ') i++
              const braceStart = s.indexOf('{', i - 2)
              if (braceStart !== -1) {
                const nameContent = extractBraced(s, braceStart)
                i = nameContent.end
                bodyId = nameContent.content.replace(/\\_/g, '_')
              }
              i = skipSpaces(s, i)
              if (s.slice(i, i + 2) === '\\!') i += 2
              i = skipSpaces(s, i)
              if (s.slice(i, i + 6) === '\\left(') {
                i += 6
                const { end } = extractLeftRight(s, i, ')')
                i = end
              }
            }
            result += `${agg}_loop("${bodyId}", ${iter}, ${startExpr}, ${endExpr})`
            break
          }
          if (s.slice(i, i + 6) === '\\left(') {
            i += 6
            const { content, end } = extractLeftRight(s, i, ')')
            i = end
            const args = splitTopLevelCommas(content).map(transformLatex).join(', ')
            result += `${name.content}(${args})`
          } else if (s[i] === '(') {
            i++ // consume '('
            // Find matching ')'
            let depth = 1
            let args = ''
            while (i < s.length && depth > 0) {
              if (s[i] === '(') depth++
              else if (s[i] === ')') { depth--; if (depth === 0) { i++; break } }
              args += s[i++]
            }
            const transformedArgs = splitTopLevelCommas(args).map(transformLatex).join(', ')
            result += `${name.content}(${transformedArgs})`
          } else {
            result += `${name.content}()`
          }
          break
        }

        case 'ln': {
          i = skipSpaces(s, i)
          if (s.slice(i, i + 6) === '\\left(') {
            i += 6
            const { content, end } = extractLeftRight(s, i, ')')
            i = end
            const args = splitTopLevelCommas(content).map(transformLatex).join(', ')
            result += `ln(${args})`
          } else {
            result += 'ln'
          }
          break
        }

        case 'exp': {
          i = skipSpaces(s, i)
          if (s.slice(i, i + 6) === '\\left(') {
            i += 6
            const { content, end } = extractLeftRight(s, i, ')')
            i = end
            result += `exp(${transformLatex(content)})`
          } else {
            result += 'exp'
          }
          break
        }

        case 'left': {
          // \left followed by delimiter: (, |, [, .
          const delim = s[i]
          i++
          if (delim === '(') {
            const { content, end } = extractLeftRight(s, i, ')')
            i = end
            result += `(${transformLatex(content)})`
          } else if (delim === '|') {
            const { content, end } = extractLeftRight(s, i, '|')
            i = end
            result += `abs(${transformLatex(content)})`
          } else {
            // Unknown left delimiter, pass through
            result += `\\left${delim}`
          }
          break
        }

        case 'right': {
          // Should be consumed by \left handler; skip the delimiter
          if (i < s.length) i++
          break
        }

        case 'cdot':
        case 'times':
          result += ' * '
          break

        case 'div':
          result += ' / '
          break

        case 'ge':
        case 'geq':
          result += ' >= '
          break

        case 'le':
        case 'leq':
          result += ' <= '
          break

        case 'ne':
        case 'neq':
          result += ' != '
          break

        case 'begin': {
          i = skipSpaces(s, i)
          const env = extractBraced(s, i)
          i = env.end
          if (env.content === 'cases') {
            const endIdx = findMatchingEndCases(s, i)
            if (endIdx === -1) break
            const casesBody = s.slice(i, endIdx)
            i = endIdx + '\\end{cases}'.length
            result += transformCases(casesBody)
          } else if (env.content === 'aligned') {
            const endTag = '\\end{aligned}'
            const endIdx = s.indexOf(endTag, i)
            if (endIdx === -1) break
            const alignedBody = s.slice(i, endIdx)
            i = endIdx + endTag.length
            const lines = alignedBody
              .split('\\\\')
              .map((l) => transformLatex(l.replace(/&/g, '').trim()))
              .filter(Boolean)
            result += lines.join('\n')
          }
          break
        }

        case 'end': {
          i = skipSpaces(s, i)
          const env2 = extractBraced(s, i)
          i = env2.end
          break
        }

        default:
          // Unknown command — pass through so partial LaTeX is visible
          result += `\\${cmd}`
      }
      continue
    }

    // `^` power
    if (c === '^') {
      i++
      i = skipSpaces(s, i)
      if (i < s.length && s[i] === '{') {
        const arg = extractBraced(s, i)
        i = arg.end
        result += ` ^ (${transformLatex(arg.content)})`
      } else if (i < s.length) {
        result += ` ^ ${s[i]}`
        i++
      }
      continue
    }

    // `{` bare grouping (treat like parentheses)
    if (c === '{') {
      const { content, end } = extractBraced(s, i)
      i = end
      const inner = transformLatex(content)
      // Only add parens if the group isn't the whole expression
      result += `(${inner})`
      continue
    }

    // `}` should be consumed by extractBraced; skip stray ones
    if (c === '}') {
      i++
      continue
    }

    // Numbers
    if (/[0-9]/.test(c) || (c === '.' && i + 1 < s.length && /[0-9]/.test(s[i + 1]))) {
      let num = ''
      while (i < s.length && /[0-9.]/.test(s[i])) { num += s[i++] }
      result += num
      continue
    }

    // Identifiers — also handles special case `e^{...}` → exp(...)
    if (/[A-Za-z_]/.test(c)) {
      let ident = ''
      while (i < s.length && /[A-Za-z0-9_]/.test(s[i])) { ident += s[i++] }

      // `e^{...}` → exp(...)
      if (ident === 'e') {
        const j = skipSpaces(s, i)
        if (j < s.length && s[j] === '^') {
          const k = skipSpaces(s, j + 1)
          if (k < s.length && s[k] === '{') {
            const arg = extractBraced(s, k)
            i = arg.end
            result += `exp(${transformLatex(arg.content)})`
            continue
          }
        }
      }

      result += ident
      continue
    }

    // Everything else: operators (+, -, *, /, =, <, >, (, ), comma, etc.)
    result += c
    i++
  }

  return result
}

/**
 * Normalize the formula text output:
 * - Collapse multiple spaces
 * - Fix spacing around operators
 */
function normalize(text: string): string {
  return text
    .replace(/\s+/g, ' ')
    .replace(/\(\s+/g, '(')
    .replace(/\s+\)/g, ')')
    .replace(/,\s+/g, ', ')
    .trim()
}

/**
 * Convert a LaTeX expression to formula text.
 *
 * Handles the subset produced by formulaTextToLatex() plus common hand-written LaTeX.
 * The result can be passed to the backend parse API.
 *
 * @throws {Error} if conversion produces empty output for non-empty input
 */
export function latexToFormulaText(latex: string): string {
  const trimmed = latex.trim()
  if (!trimmed) return ''

  // Handle multi-line input: split by actual newlines (not LaTeX \\),
  // unless it's an \begin{...} block
  if (!trimmed.includes('\\begin')) {
    const lines = trimmed.split('\n').map((l) => l.trim()).filter(Boolean)
    if (lines.length > 1) {
      return lines.map((l) => normalize(transformLatex(l))).join('\n')
    }
  }

  return normalize(transformLatex(trimmed))
}
