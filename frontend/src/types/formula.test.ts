import { describe, expectTypeOf, it } from 'vitest'
import type { FunctionKind, NodeType } from './formula'

describe('formula wire-format type contract', () => {
  it('matches the backend-supported function and node discriminators', () => {
    expectTypeOf<FunctionKind>().toEqualTypeOf<
      | 'round'
      | 'ceil'
      | 'floor'
      | 'abs'
      | 'min'
      | 'max'
      | 'sqrt'
      | 'ln'
      | 'exp'
    >()

    expectTypeOf<NodeType>().toEqualTypeOf<
      | 'variable'
      | 'constant'
      | 'operator'
      | 'function'
      | 'subFormula'
      | 'tableLookup'
      | 'tableAggregate'
      | 'conditional'
      | 'aggregate'
      | 'loop'
    >()
  })
})
