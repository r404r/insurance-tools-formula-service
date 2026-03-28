import BigNumber from 'bignumber.js'

/**
 * Format a high-precision decimal string for display.
 * Trims trailing zeros and uses fixed notation for reasonable ranges,
 * or exponential notation for very large/small numbers.
 */
export function formatDecimal(value: string, displayPrecision?: number): string {
  const bn = new BigNumber(value)
  if (bn.isNaN()) return value
  if (!bn.isFinite()) return bn.isPositive() ? '+Inf' : '-Inf'

  if (displayPrecision !== undefined) {
    return bn.toFixed(displayPrecision)
  }

  const abs = bn.abs()
  if (abs.isZero()) return '0'
  if (abs.isGreaterThanOrEqualTo(1e15) || abs.isLessThan(1e-6)) {
    return bn.toExponential()
  }

  return bn.toFormat({
    groupSize: 3,
    groupSeparator: ',',
    decimalSeparator: '.',
  })
}

/**
 * Format a percentage value (0-1 scale) for display.
 */
export function formatPercent(value: string, decimalPlaces: number = 2): string {
  const bn = new BigNumber(value)
  if (bn.isNaN()) return value
  return bn.multipliedBy(100).toFixed(decimalPlaces) + '%'
}

/**
 * Format a currency value with a symbol prefix.
 */
export function formatCurrency(value: string, symbol: string = '\u00a5', decimalPlaces: number = 2): string {
  const bn = new BigNumber(value)
  if (bn.isNaN()) return value
  const formatted = bn.toFormat(decimalPlaces, {
    groupSize: 3,
    groupSeparator: ',',
    decimalSeparator: '.',
  })
  return `${symbol}${formatted}`
}

/**
 * Compare two high-precision values.
 * Returns -1 if a < b, 0 if equal, 1 if a > b.
 */
export function compareValues(a: string, b: string): -1 | 0 | 1 {
  const result = new BigNumber(a).comparedTo(new BigNumber(b))
  if (result === null) return 0
  return result as -1 | 0 | 1
}
