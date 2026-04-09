type TokenType =
  | 'number'
  | 'identifier'
  | 'string'
  | 'operator'
  | 'lparen'
  | 'rparen'
  | 'comma'
  | 'if'
  | 'then'
  | 'else'
  | 'eof'

interface Token {
  type: TokenType
  value: string
}

type AstNode =
  | { kind: 'number'; value: string }
  | { kind: 'identifier'; value: string }
  | { kind: 'string'; value: string }
  | { kind: 'unary'; operator: string; operand: AstNode }
  | { kind: 'binary'; operator: string; left: AstNode; right: AstNode }
  | { kind: 'call'; name: string; args: AstNode[] }
  | { kind: 'conditional'; condition: AstNode; thenBranch: AstNode; elseBranch: AstNode }

const operatorPrecedence: Record<string, number> = {
  '==': 1,
  '!=': 1,
  '>': 1,
  '<': 1,
  '>=': 1,
  '<=': 1,
  '+': 2,
  '-': 2,
  '*': 3,
  '/': 3,
  '%': 3,
  '^': 4,
}

function tokenize(input: string): Token[] {
  const tokens: Token[] = []
  let index = 0

  while (index < input.length) {
    const char = input[index]

    if (/\s/.test(char)) {
      index += 1
      continue
    }

    const twoChar = input.slice(index, index + 2)
    if (['>=', '<=', '==', '!='].includes(twoChar)) {
      tokens.push({ type: 'operator', value: twoChar })
      index += 2
      continue
    }

    if ('+-*/^%><'.includes(char)) {
      tokens.push({ type: 'operator', value: char })
      index += 1
      continue
    }

    if (char === '(') {
      tokens.push({ type: 'lparen', value: char })
      index += 1
      continue
    }

    if (char === ')') {
      tokens.push({ type: 'rparen', value: char })
      index += 1
      continue
    }

    if (char === ',') {
      tokens.push({ type: 'comma', value: char })
      index += 1
      continue
    }

    if (char === '"') {
      let cursor = index + 1
      let value = ''
      while (cursor < input.length) {
        const next = input[cursor]
        if (next === '\\' && cursor + 1 < input.length) {
          value += input[cursor + 1]
          cursor += 2
          continue
        }
        if (next === '"') {
          break
        }
        value += next
        cursor += 1
      }
      if (cursor >= input.length || input[cursor] !== '"') {
        throw new Error('Unterminated string literal')
      }
      tokens.push({ type: 'string', value })
      index = cursor + 1
      continue
    }

    if (/[0-9.]/.test(char)) {
      let cursor = index + 1
      while (cursor < input.length && /[0-9.]/.test(input[cursor])) {
        cursor += 1
      }
      tokens.push({ type: 'number', value: input.slice(index, cursor) })
      index = cursor
      continue
    }

    if (/[A-Za-z_]/.test(char)) {
      let cursor = index + 1
      while (cursor < input.length && /[A-Za-z0-9_]/.test(input[cursor])) {
        cursor += 1
      }
      const value = input.slice(index, cursor)
      if (value === 'if' || value === 'then' || value === 'else') {
        tokens.push({ type: value, value })
      } else {
        tokens.push({ type: 'identifier', value })
      }
      index = cursor
      continue
    }

    throw new Error(`Unexpected character "${char}"`)
  }

  tokens.push({ type: 'eof', value: '' })
  return tokens
}

class Parser {
  private readonly tokens: Token[]
  private index = 0

  constructor(input: string) {
    this.tokens = tokenize(input)
  }

  parse(): AstNode {
    const expression = this.parseExpression()
    this.expect('eof')
    return expression
  }

  private current(): Token {
    return this.tokens[this.index]
  }

  private advance(): Token {
    const token = this.tokens[this.index]
    this.index += 1
    return token
  }

  private expect(type: TokenType): Token {
    const token = this.current()
    if (token.type !== type) {
      throw new Error(`Expected ${type}, received ${token.type}`)
    }
    return this.advance()
  }

  private parseExpression(minPrecedence = 0): AstNode {
    let left = this.parsePrefix()

    while (this.current().type === 'operator') {
      const operator = this.current().value
      const precedence = operatorPrecedence[operator]
      if (precedence === undefined || precedence < minPrecedence) {
        break
      }

      this.advance()
      const nextPrecedence = operator === '^' ? precedence : precedence + 1
      const right = this.parseExpression(nextPrecedence)
      left = { kind: 'binary', operator, left, right }
    }

    return left
  }

  private parsePrefix(): AstNode {
    const token = this.current()

    if (token.type === 'operator' && token.value === '-') {
      this.advance()
      return { kind: 'unary', operator: '-', operand: this.parseExpression(5) }
    }

    if (token.type === 'if') {
      this.advance()
      const condition = this.parseExpression()
      this.expect('then')
      const thenBranch = this.parseExpression()
      this.expect('else')
      const elseBranch = this.parseExpression()
      return { kind: 'conditional', condition, thenBranch, elseBranch }
    }

    if (token.type === 'number') {
      this.advance()
      return { kind: 'number', value: token.value }
    }

    if (token.type === 'string') {
      this.advance()
      return { kind: 'string', value: token.value }
    }

    if (token.type === 'identifier') {
      this.advance()
      if (this.current().type !== 'lparen') {
        return { kind: 'identifier', value: token.value }
      }

      this.advance()
      const args: AstNode[] = []
      if (this.current().type !== 'rparen') {
        do {
          args.push(this.parseExpression())
          if (this.current().type !== 'comma') {
            break
          }
          this.advance()
        } while (this.current().type !== 'rparen')
      }
      this.expect('rparen')
      return { kind: 'call', name: token.value, args }
    }

    if (token.type === 'lparen') {
      this.advance()
      const expression = this.parseExpression()
      this.expect('rparen')
      return expression
    }

    throw new Error(`Unexpected token ${token.type}`)
  }
}

function escapeLatex(value: string): string {
  return value
    .replace(/\\/g, '\\textbackslash ')
    .replace(/_/g, '\\_')
    .replace(/%/g, '\\%')
    .replace(/\$/g, '\\$')
    .replace(/#/g, '\\#')
    .replace(/{/g, '\\{')
    .replace(/}/g, '\\}')
}

const LOOP_AGG_LATEX: Record<string, string> = {
  sum_loop: '\\sum',
  product_loop: '\\prod',
  count_loop: '\\operatorname{count}',
  avg_loop: '\\operatorname{avg}',
  min_loop: '\\min',
  max_loop: '\\max',
  last_loop: '\\operatorname{last}',
}

/** Render AGG_loop(formulaId, iterator, start, end[, step]) as proper math notation. */
function formatLoopLatex(symbol: string, args: AstNode[]): string {
  const formulaId = args[0].kind === 'string' ? args[0].value : args[0].kind === 'identifier' ? args[0].value : '?'
  const iter = args[1].kind === 'identifier' ? args[1].value : 't'
  const start = toLatex(args[2])
  const end = toLatex(args[3])
  const step = args.length >= 5 ? toLatex(args[4]) : ''
  const subscript = step ? `${iter}=${start},\\, \\Delta=${step}` : `${iter}=${start}`
  const body = `\\operatorname{${escapeLatex(formulaId)}}\\!\\left(${iter}\\right)`
  return `${symbol}_{${subscript}}^{${end}} ${body}`
}

function formatFunction(name: string, args: string[]): string {
  switch (name) {
    case 'sqrt':
      return args[0] ? `\\sqrt{${args[0]}}` : '\\sqrt{\\placeholder{}}'
    case 'abs':
      return args[0] ? `\\left|${args[0]}\\right|` : '\\left|\\placeholder{}\\right|'
    case 'ln':
      return `\\ln\\left(${args.join(', ')}\\right)`
    case 'exp':
      return args[0] ? `e^{${args[0]}}` : '\\exp'
    default:
      return `\\operatorname{${escapeLatex(name)}}\\left(${args.join(', ')}\\right)`
  }
}

function toLatex(node: AstNode, parentPrecedence = 0): string {
  switch (node.kind) {
    case 'number':
      return node.value
    case 'identifier':
      return `\\mathrm{${escapeLatex(node.value)}}`
    case 'string':
      return `\\text{${escapeLatex(node.value)}}`
    case 'unary': {
      const operand = toLatex(node.operand, 5)
      return `-${operand}`
    }
    case 'binary': {
      const precedence = operatorPrecedence[node.operator]

      if (node.operator === '/') {
        return `\\frac{${toLatex(node.left)}}{${toLatex(node.right)}}`
      }

      if (node.operator === '^') {
        const base = toLatex(node.left, precedence)
        const exponent = toLatex(node.right)
        return `${base}^{${exponent}}`
      }

      const left = toLatex(node.left, precedence)
      const right = toLatex(node.right, precedence + 1)
      let expression = ''
      switch (node.operator) {
        case '*':
          expression = `${left} \\cdot ${right}`
          break
        case '>=':
          expression = `${left} \\ge ${right}`
          break
        case '<=':
          expression = `${left} \\le ${right}`
          break
        case '!=':
          expression = `${left} \\ne ${right}`
          break
        default:
          expression = `${left} ${node.operator} ${right}`
          break
      }

      if (precedence < parentPrecedence) {
        return `\\left(${expression}\\right)`
      }
      return expression
    }
    case 'call': {
      const loopSym = LOOP_AGG_LATEX[node.name]
      if (loopSym && node.args.length >= 4) {
        return formatLoopLatex(loopSym, node.args)
      }
      return formatFunction(node.name, node.args.map((arg) => toLatex(arg)))
    }
    case 'conditional': {
      const condition = toLatex(node.condition)
      const thenBranch = toLatex(node.thenBranch)
      const elseBranch = toLatex(node.elseBranch)
      return String.raw`\begin{cases}
${thenBranch}, & \text{if } ${condition} \\
${elseBranch}, & \text{otherwise}
\end{cases}`
    }
  }
}

export function formulaTextToLatex(input: string): string {
  const trimmed = input.trim()
  if (!trimmed) {
    return ''
  }

  const expressions = trimmed
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line) => {
      // Only strip "identifier =" prefix; avoid false matches on >=, <=, !=, ==.
      // Use Unicode property escapes (\p{L}, \p{N}) so non-ASCII labels like
      // "保费 = age * rate" are handled correctly.
      const assignmentMatch = /^[\p{L}_][\p{L}\p{N}_]*\s*=(?!=)/u.exec(line)
      return assignmentMatch ? line.slice(assignmentMatch[0].length).trim() : line
    })

  const rendered = expressions.map((expression) => {
    const parser = new Parser(expression)
    return toLatex(parser.parse())
  })

  if (rendered.length === 1) {
    return rendered[0]
  }

  return String.raw`\begin{aligned}${rendered.map((item) => item).join(String.raw` \\ `)}\end{aligned}`
}
