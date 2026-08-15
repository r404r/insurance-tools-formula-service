import { useEffect, useRef } from 'react'
import { useBeforeUnload } from 'react-router-dom'

function historyIndex(state: unknown): number | null {
  if (!state || typeof state !== 'object') return null
  const idx = (state as { idx?: unknown }).idx
  return typeof idx === 'number' && Number.isFinite(idx) ? idx : null
}

export function useUnsavedChangesGuard(dirty: boolean, message: string) {
  const restoringHistoryRef = useRef(false)
  const currentHistoryIndexRef = useRef<number | null>(historyIndex(window.history.state))
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

  // BrowserRouter listens to popstate but, unlike a data router, cannot block
  // it before the browser changes history. React Router stores a monotonically
  // increasing `idx` in history.state; use the delta to compensate in the
  // opposite direction for both Back and Forward. The +1 fallback preserves
  // the old, conservative Back behavior for external entries without an idx.
  useEffect(() => {
    if (!dirty) return
    currentHistoryIndexRef.current = historyIndex(window.history.state)

    const handlePopState = (event: PopStateEvent) => {
      const destinationIndex = historyIndex(event.state) ?? historyIndex(window.history.state)
      if (restoringHistoryRef.current) {
        restoringHistoryRef.current = false
        currentHistoryIndexRef.current = destinationIndex
        return
      }
      if (!window.confirm(message)) {
        restoringHistoryRef.current = true
        const currentIndex = currentHistoryIndexRef.current
        const delta = currentIndex !== null && destinationIndex !== null
          ? currentIndex - destinationIndex
          : 1
        window.history.go(delta === 0 ? 1 : delta)
        return
      }
      currentHistoryIndexRef.current = destinationIndex
    }

    window.addEventListener('popstate', handlePopState)
    return () => window.removeEventListener('popstate', handlePopState)
  }, [dirty, message])
}
