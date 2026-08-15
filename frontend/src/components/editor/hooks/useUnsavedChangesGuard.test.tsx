// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render } from '@testing-library/react'

const mocks = vi.hoisted(() => ({ useBeforeUnload: vi.fn() }))

vi.mock('react-router-dom', () => ({
  useBeforeUnload: mocks.useBeforeUnload,
}))

import { useUnsavedChangesGuard } from './useUnsavedChangesGuard'

afterEach(cleanup)

function Guard({ dirty }: { dirty: boolean }) {
  useUnsavedChangesGuard(dirty, 'You have unsaved changes. Leave without saving?')
  return <a href="/other" onClick={(event) => event.preventDefault()}>Other page</a>
}

describe('useUnsavedChangesGuard', () => {
  it('protects both route navigation and browser unload while graph changes are unsaved', () => {
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    const { getByRole } = render(<Guard dirty />)

    expect(mocks.useBeforeUnload).toHaveBeenCalledWith(expect.any(Function), { capture: true })
    expect(fireEvent.click(getByRole('link'))).toBe(false)
    expect(confirm).toHaveBeenCalledWith('You have unsaved changes. Leave without saving?')
    confirm.mockRestore()
  })

  it('does not block navigation after the graph is saved', () => {
    const confirm = vi.spyOn(window, 'confirm')
    const { getByRole } = render(<Guard dirty={false} />)

    fireEvent.click(getByRole('link'))
    expect(confirm).not.toHaveBeenCalled()
    confirm.mockRestore()
  })
})
