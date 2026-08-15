// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import type { Node } from '@xyflow/react'

vi.mock('@tanstack/react-query', () => ({
  useQuery: () => ({ data: [] }),
}))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

import NodePropertiesPanel from './NodePropertiesPanel'

afterEach(cleanup)

function functionNode(fn: 'floor' | 'ceil'): Node {
  return {
    id: `${fn}_precision`,
    type: 'formulaNode',
    position: { x: 0, y: 0 },
    data: {
      nodeType: 'function',
      config: { fn, args: { places: '2' } },
    },
  } as unknown as Node
}

describe('NodePropertiesPanel floor/ceil precision configuration', () => {
  it.each(['floor', 'ceil'] as const)('exposes and persists the places setting for %s', (fn) => {
    const onChange = vi.fn()
    render(<NodePropertiesPanel node={functionNode(fn)} onChange={onChange} />)

    const places = screen.getByPlaceholderText('18') as HTMLInputElement
    expect(places.value).toBe('2')

    fireEvent.change(places, { target: { value: '3' } })

    expect(onChange).toHaveBeenCalledWith(`${fn}_precision`, {
      nodeType: 'function',
      config: { fn, args: { places: '3' } },
    })
  })
})
