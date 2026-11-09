import { useEffect, useRef, useState } from 'react'
import { useLocation } from 'react-router-dom'

// The left menu as a drawer on phones and tablets: the menu button opens and
// shuts it, and any change of page closes it. While it is open, Escape closes
// it, Tab stays inside it, and focus goes back to the menu button when it
// closes. Both layouts (the full app and the one for an account that needs a
// subscription) use it, so the drawer works the same in each.
export function useMenuDrawer() {
  const [menuOpen, setMenuOpen] = useState(false)
  const location = useLocation()
  const navRef = useRef<HTMLElement>(null)
  const menuButton = useRef<HTMLButtonElement>(null)

  // Close the mobile drawer whenever the page changes (a Library line only changes the address's query).
  useEffect(() => {
    setMenuOpen(false)
  }, [location.pathname, location.search])

  useEffect(() => {
    if (!menuOpen) return
    const nav = navRef.current
    if (!nav || !window.matchMedia('(max-width: 900px)').matches) return
    const items = () => [...nav.querySelectorAll<HTMLElement>('a[href], button:not([disabled]), input:not([disabled]), select:not([disabled])')].filter((el) => el.getClientRects().length > 0)
    items()[0]?.focus({ preventScroll: true })
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        setMenuOpen(false)
        menuButton.current?.focus({ preventScroll: true })
        return
      }
      if (e.key !== 'Tab') return
      const list = items()
      if (list.length === 0) return
      const first = list[0]
      const last = list[list.length - 1]
      const at = document.activeElement
      if (e.shiftKey && (at === first || !nav.contains(at))) {
        e.preventDefault()
        last.focus()
      } else if (!e.shiftKey && (at === last || !nav.contains(at))) {
        e.preventDefault()
        first.focus()
      }
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [menuOpen])

  return { menuOpen, setMenuOpen, navRef, menuButton }
}
