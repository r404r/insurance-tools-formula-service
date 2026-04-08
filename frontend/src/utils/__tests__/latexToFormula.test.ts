/**
 * Task #018 — LaTeX Formula Input Test Suite
 *
 * 20 test cases covering all supported LaTeX syntax features in latexToFormulaText().
 * Each case verifies the LaTeX → formula text conversion that feeds into the DAG parser.
 */

import { describe, it, expect } from 'vitest'
import { latexToFormulaText } from '../latexToFormula'

// ---------------------------------------------------------------------------
// TC-01: Variable identifiers with \mathrm and addition
// ---------------------------------------------------------------------------
describe('TC-01: Addition of variables with mathrm', () => {
  it('converts mathrm identifiers and + operator', () => {
    expect(latexToFormulaText('\\mathrm{age} + \\mathrm{base\\_rate}')).toBe('age + base_rate')
  })

  it('handles simple numeric addition', () => {
    expect(latexToFormulaText('10 + 20')).toBe('10 + 20')
  })

  it('handles mixed variable and constant', () => {
    expect(latexToFormulaText('\\mathrm{x} + 5')).toBe('x + 5')
  })
})

// ---------------------------------------------------------------------------
// TC-02: Subtraction
// ---------------------------------------------------------------------------
describe('TC-02: Subtraction', () => {
  it('converts two mathrm variables subtracted', () => {
    expect(latexToFormulaText('\\mathrm{premium} - \\mathrm{discount}')).toBe('premium - discount')
  })

  it('handles numeric subtraction', () => {
    expect(latexToFormulaText('100 - 25')).toBe('100 - 25')
  })

  it('handles negative constant', () => {
    expect(latexToFormulaText('-\\mathrm{x}')).toBe('-x')
  })
})

// ---------------------------------------------------------------------------
// TC-03: Multiplication with \cdot
// ---------------------------------------------------------------------------
describe('TC-03: \\cdot → *', () => {
  it('converts \\cdot to *', () => {
    expect(latexToFormulaText('\\mathrm{sum\\_insured} \\cdot \\mathrm{rate}')).toBe('sum_insured * rate')
  })

  it('handles multiple \\cdot operators', () => {
    expect(latexToFormulaText('\\mathrm{a} \\cdot \\mathrm{b} \\cdot \\mathrm{c}')).toBe('a * b * c')
  })

  it('handles constant multiplied by variable', () => {
    expect(latexToFormulaText('2 \\cdot \\mathrm{x}')).toBe('2 * x')
  })
})

// ---------------------------------------------------------------------------
// TC-04: Multiplication with \times
// ---------------------------------------------------------------------------
describe('TC-04: \\times → *', () => {
  it('converts \\times to *', () => {
    expect(latexToFormulaText('\\mathrm{principal} \\times \\mathrm{factor}')).toBe('principal * factor')
  })

  it('handles \\times with constants', () => {
    expect(latexToFormulaText('3 \\times 4')).toBe('3 * 4')
  })

  it('handles mixed \\cdot and \\times', () => {
    expect(latexToFormulaText('\\mathrm{a} \\cdot \\mathrm{b} \\times \\mathrm{c}')).toBe('a * b * c')
  })
})

// ---------------------------------------------------------------------------
// TC-05: Division with \frac
// ---------------------------------------------------------------------------
describe('TC-05: \\frac{a}{b} → (a) / (b)', () => {
  it('converts \\frac with variable numerator and constant denominator', () => {
    expect(latexToFormulaText('\\frac{\\mathrm{annual}}{12}')).toBe('(annual) / (12)')
  })

  it('converts \\frac with two variables', () => {
    expect(latexToFormulaText('\\frac{\\mathrm{a}}{\\mathrm{b}}')).toBe('(a) / (b)')
  })

  it('converts nested \\frac (numerator is expression)', () => {
    expect(latexToFormulaText('\\frac{\\mathrm{a} + \\mathrm{b}}{2}')).toBe('(a + b) / (2)')
  })
})

// ---------------------------------------------------------------------------
// TC-06: Power operator ^{...}
// ---------------------------------------------------------------------------
describe('TC-06: x^{n} → x ^ (n)', () => {
  it('converts simple power', () => {
    expect(latexToFormulaText('\\mathrm{x}^{2}')).toBe('x ^ (2)')
  })

  it('converts power with variable exponent', () => {
    expect(latexToFormulaText('\\mathrm{base}^{\\mathrm{n}}')).toBe('base ^ (n)')
  })

  it('converts power with expression in exponent', () => {
    expect(latexToFormulaText('\\mathrm{x}^{\\mathrm{a} + \\mathrm{b}}')).toBe('x ^ (a + b)')
  })
})

// ---------------------------------------------------------------------------
// TC-07: Natural exponent e^{...} → exp(...)
// ---------------------------------------------------------------------------
describe('TC-07: e^{x} → exp(x)', () => {
  it('converts e^{variable} to exp(variable)', () => {
    expect(latexToFormulaText('e^{\\mathrm{r}}')).toBe('exp(r)')
  })

  it('converts e^{expression} to exp(expression)', () => {
    expect(latexToFormulaText('e^{\\mathrm{r} \\cdot \\mathrm{t}}')).toBe('exp(r * t)')
  })

  it('converts \\exp\\left(x\\right) to exp(x)', () => {
    expect(latexToFormulaText('\\exp\\left(\\mathrm{x}\\right)')).toBe('exp(x)')
  })
})

// ---------------------------------------------------------------------------
// TC-08: Square root \sqrt{...}
// ---------------------------------------------------------------------------
describe('TC-08: \\sqrt{x} → sqrt(x)', () => {
  it('converts \\sqrt with variable', () => {
    expect(latexToFormulaText('\\sqrt{\\mathrm{x}}')).toBe('sqrt(x)')
  })

  it('converts \\sqrt with expression', () => {
    expect(latexToFormulaText('\\sqrt{\\mathrm{a}^{2} + \\mathrm{b}^{2}}')).toBe('sqrt(a ^ (2) + b ^ (2))')
  })

  it('throws on nth-root \\sqrt[n]{...}', () => {
    expect(() => latexToFormulaText('\\sqrt[3]{\\mathrm{x}}')).toThrow(/nth-root/)
  })
})

// ---------------------------------------------------------------------------
// TC-09: Absolute value \left|...\right|
// ---------------------------------------------------------------------------
describe('TC-09: \\left|x\\right| → abs(x)', () => {
  it('converts absolute value of variable', () => {
    expect(latexToFormulaText('\\left|\\mathrm{x}\\right|')).toBe('abs(x)')
  })

  it('converts absolute value of expression', () => {
    expect(latexToFormulaText('\\left|\\mathrm{a} - \\mathrm{b}\\right|')).toBe('abs(a - b)')
  })

  it('converts nested abs inside expression', () => {
    expect(latexToFormulaText('\\left|\\mathrm{x}\\right| + 1')).toBe('abs(x) + 1')
  })
})

// ---------------------------------------------------------------------------
// TC-10: Natural log \ln\left(...\right)
// ---------------------------------------------------------------------------
describe('TC-10: \\ln\\left(x\\right) → ln(x)', () => {
  it('converts \\ln with \\left(...\\right)', () => {
    expect(latexToFormulaText('\\ln\\left(\\mathrm{x}\\right)')).toBe('ln(x)')
  })

  it('converts \\ln with expression argument', () => {
    expect(latexToFormulaText('\\ln\\left(\\mathrm{a} + \\mathrm{b}\\right)')).toBe('ln(a + b)')
  })

  it('keeps bare \\ln as identifier when no args follow', () => {
    expect(latexToFormulaText('\\ln')).toBe('ln')
  })
})

// ---------------------------------------------------------------------------
// TC-11: Custom function via \operatorname{max}
// ---------------------------------------------------------------------------
describe('TC-11: \\operatorname{max}\\left(a, b\\right) → max(a, b)', () => {
  it('converts max function', () => {
    expect(latexToFormulaText('\\operatorname{max}\\left(\\mathrm{a}, \\mathrm{b}\\right)')).toBe('max(a, b)')
  })

  it('converts max with constants', () => {
    expect(latexToFormulaText('\\operatorname{max}\\left(0, \\mathrm{x}\\right)')).toBe('max(0, x)')
  })

  it('converts max with expression arguments', () => {
    expect(latexToFormulaText('\\operatorname{max}\\left(\\mathrm{a} + 1, \\mathrm{b} - 1\\right)')).toBe('max(a + 1, b - 1)')
  })
})

// ---------------------------------------------------------------------------
// TC-12: min function via \operatorname{min}
// ---------------------------------------------------------------------------
describe('TC-12: \\operatorname{min}\\left(lo, hi\\right) → min(lo, hi)', () => {
  it('converts min function', () => {
    expect(latexToFormulaText('\\operatorname{min}\\left(\\mathrm{lo}, \\mathrm{hi}\\right)')).toBe('min(lo, hi)')
  })

  it('converts min with constant cap', () => {
    expect(latexToFormulaText('\\operatorname{min}\\left(\\mathrm{x}, 100\\right)')).toBe('min(x, 100)')
  })

  it('converts min with fractional constant', () => {
    expect(latexToFormulaText('\\operatorname{min}\\left(\\mathrm{rate}, 0.05\\right)')).toBe('min(rate, 0.05)')
  })
})

// ---------------------------------------------------------------------------
// TC-13: Parentheses grouping \left(...\right)
// ---------------------------------------------------------------------------
describe('TC-13: \\left(a + b\\right) \\cdot c → (a + b) * c', () => {
  it('converts parenthesized addition times variable', () => {
    expect(latexToFormulaText('\\left(\\mathrm{a} + \\mathrm{b}\\right) \\cdot \\mathrm{c}')).toBe('(a + b) * c')
  })

  it('converts nested parentheses', () => {
    expect(latexToFormulaText('\\left(\\left(\\mathrm{a} + \\mathrm{b}\\right) \\cdot \\mathrm{c}\\right)')).toBe('((a + b) * c)')
  })

  it('converts parenthesized sum in denominator', () => {
    expect(latexToFormulaText('\\frac{\\mathrm{x}}{\\left(\\mathrm{a} + \\mathrm{b}\\right)}')).toBe('(x) / ((a + b))')
  })
})

// ---------------------------------------------------------------------------
// TC-14: Comparison >= via \ge — in conditional context
// ---------------------------------------------------------------------------
describe('TC-14: \\ge → >= in conditional', () => {
  it('converts \\ge in cases environment', () => {
    const latex = `\\begin{cases}\n\\mathrm{premium} \\cdot 1.5, & \\text{if } \\mathrm{age} \\ge 60 \\\\\n\\mathrm{premium}, & \\text{otherwise}\n\\end{cases}`
    expect(latexToFormulaText(latex)).toBe('if age >= 60 then premium * 1.5 else premium')
  })

  it('converts bare \\ge to >=', () => {
    expect(latexToFormulaText('\\mathrm{age} \\ge 18')).toBe('age >= 18')
  })

  it('converts \\geq (alternative form) to >=', () => {
    expect(latexToFormulaText('\\mathrm{x} \\geq \\mathrm{y}')).toBe('x >= y')
  })
})

// ---------------------------------------------------------------------------
// TC-15: Comparison <= via \le — in conditional context
// ---------------------------------------------------------------------------
describe('TC-15: \\le → <= in conditional', () => {
  it('converts \\le in cases environment', () => {
    const latex = `\\begin{cases}\n\\mathrm{premium} \\cdot 0.9, & \\text{if } \\mathrm{risk} \\le 0.3 \\\\\n\\mathrm{premium}, & \\text{otherwise}\n\\end{cases}`
    expect(latexToFormulaText(latex)).toBe('if risk <= 0.3 then premium * 0.9 else premium')
  })

  it('converts bare \\le to <=', () => {
    expect(latexToFormulaText('\\mathrm{risk} \\le 0.5')).toBe('risk <= 0.5')
  })

  it('converts \\leq (alternative form) to <=', () => {
    expect(latexToFormulaText('\\mathrm{x} \\leq 100')).toBe('x <= 100')
  })
})

// ---------------------------------------------------------------------------
// TC-16: Comparison != via \ne / \neq — in conditional context
// ---------------------------------------------------------------------------
describe('TC-16: \\ne / \\neq → != in conditional', () => {
  it('converts \\ne in cases environment (bonus flag)', () => {
    const latex = `\\begin{cases}\n\\mathrm{bonus}, & \\text{if } \\mathrm{flag} \\ne 0 \\\\\n0, & \\text{otherwise}\n\\end{cases}`
    expect(latexToFormulaText(latex)).toBe('if flag != 0 then bonus else 0')
  })

  it('converts bare \\ne to !=', () => {
    expect(latexToFormulaText('\\mathrm{status} \\ne 0')).toBe('status != 0')
  })

  it('converts \\neq (alternative form) to !=', () => {
    expect(latexToFormulaText('\\mathrm{x} \\neq \\mathrm{y}')).toBe('x != y')
  })
})

// ---------------------------------------------------------------------------
// TC-17: Alternative comparison forms \geq, \leq, \neq (alias coverage)
// ---------------------------------------------------------------------------
describe('TC-17: alternative \\geq / \\leq / \\neq forms', () => {
  it('\\geq converts to >= in conditional', () => {
    const latex = `\\begin{cases}\n100, & \\text{if } \\mathrm{score} \\geq 90 \\\\\n\\mathrm{score}, & \\text{otherwise}\n\\end{cases}`
    expect(latexToFormulaText(latex)).toBe('if score >= 90 then 100 else score')
  })

  it('\\leq converts to <=', () => {
    expect(latexToFormulaText('\\mathrm{a} \\leq \\mathrm{b}')).toBe('a <= b')
  })

  it('\\neq converts to !=', () => {
    expect(latexToFormulaText('\\mathrm{a} \\neq \\mathrm{b}')).toBe('a != b')
  })
})

// ---------------------------------------------------------------------------
// TC-18: Simple conditional via \begin{cases}...\end{cases}
// ---------------------------------------------------------------------------
describe('TC-18: \\begin{cases} → if...then...else', () => {
  it('converts simple two-branch conditional', () => {
    const latex = `\\begin{cases}\n1.5, & \\text{if } \\mathrm{age} \\ge 65 \\\\\n1, & \\text{otherwise}\n\\end{cases}`
    expect(latexToFormulaText(latex)).toBe('if age >= 65 then 1.5 else 1')
  })

  it('converts conditional with expression in then-branch', () => {
    const latex = `\\begin{cases}\n\\mathrm{premium} \\cdot 1.5, & \\text{if } \\mathrm{risk} \\ge 0.8 \\\\\n\\mathrm{premium}, & \\text{otherwise}\n\\end{cases}`
    expect(latexToFormulaText(latex)).toBe('if risk >= 0.8 then premium * 1.5 else premium')
  })

  it('converts conditional with equality comparison', () => {
    const latex = `\\begin{cases}\n0, & \\text{if } \\mathrm{x} \\le 0 \\\\\n\\mathrm{x}, & \\text{otherwise}\n\\end{cases}`
    expect(latexToFormulaText(latex)).toBe('if x <= 0 then 0 else x')
  })
})

// ---------------------------------------------------------------------------
// TC-19: Nested conditional (nested \begin{cases})
// ---------------------------------------------------------------------------
describe('TC-19: nested \\begin{cases} → nested if...then...else', () => {
  it('converts two-level nested conditional', () => {
    const inner = `\\begin{cases}\n\\mathrm{b}, & \\text{if } \\mathrm{y} \\ge 0 \\\\\n\\mathrm{c}, & \\text{otherwise}\n\\end{cases}`
    const outer = `\\begin{cases}\n\\mathrm{a}, & \\text{if } \\mathrm{x} \\ge 0 \\\\\n${inner}, & \\text{otherwise}\n\\end{cases}`
    expect(latexToFormulaText(outer)).toBe('if x >= 0 then a else if y >= 0 then b else c')
  })
})

// ---------------------------------------------------------------------------
// TC-20: Complex compound formula (multi-operator precedence)
// ---------------------------------------------------------------------------
describe('TC-20: Complex compound formula', () => {
  it('converts sum_insured * rate / 1000', () => {
    expect(latexToFormulaText('\\frac{\\mathrm{sum\\_insured} \\cdot \\mathrm{rate}}{1000}')).toBe('(sum_insured * rate) / (1000)')
  })

  it('converts compound with sqrt and power', () => {
    expect(latexToFormulaText('\\sqrt{\\mathrm{a}^{2} + \\mathrm{b}^{2}}')).toBe('sqrt(a ^ (2) + b ^ (2))')
  })

  it('converts multi-term insurance premium formula', () => {
    const latex = '\\mathrm{base\\_premium} \\cdot \\left(1 + \\mathrm{loading}\\right)'
    expect(latexToFormulaText(latex)).toBe('base_premium * (1 + loading)')
  })
})
