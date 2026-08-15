import { useEffect } from 'react'
import { useBeforeUnload } from 'react-router-dom'

export function useUnsavedChangesGuard(dirty: boolean, message: string) {
  useBeforeUnload(
    (event) => {
      if (!dirty) return
      event.preventDefault()
      event.returnValue = ''
    },
    { capture: true },
  )

  // BrowserRouter is not a data router, so react-router's useBlocker/usePrompt
  // cannot be used here. Capture same-origin link clicks before Link handles
  // them and let the user cancel the route transition.
  useEffect(() => {
    if (!dirty) return

    const handleClick = (event: MouseEvent) => {
      if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) {
        return
      }
      const target = event.target instanceof Element ? event.target.closest('a[href]') : null
      if (!(target instanceof HTMLAnchorElement) || target.target || target.hasAttribute('download')) return
      const destination = new URL(target.href, window.location.href)
      if (destination.origin !== window.location.origin || destination.href === window.location.href) return
      if (!window.confirm(message)) {
        event.preventDefault()
        event.stopPropagation()
      }
    }

    document.addEventListener('click', handleClick, true)
    return () => document.removeEventListener('click', handleClick, true)
  }, [dirty, message])
}
