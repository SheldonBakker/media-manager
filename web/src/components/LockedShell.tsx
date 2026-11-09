import { Suspense } from 'react'
import { NavLink, Outlet, useLocation } from 'react-router-dom'
import { subscriptionNav } from '../api'
import { useAuth } from '../AuthContext'
import { settingsGroupsFor } from '../settingsNav'
import { t, tc } from '../i18n'
import BrandMark from './BrandMark'
import Icon from './Icon'
import ProfileMenu from './ProfileMenu'
import ThemeToggle from './ThemeToggle'
import PageLoading from './PageLoading'
import { useMenuDrawer } from './useMenuDrawer'

// What a member sees while their access waits on a subscription: the brand,
// the three Account & Access pages and a way to sign out. It leaves out
// everything the rest of the app loads (downloads, search, the music player),
// so nothing there asks the server for what this account may not open. On a
// phone the menu is the same drawer as in the full app.
export default function LockedShell() {
  const { user } = useAuth()
  const { pathname } = useLocation()
  const { menuOpen, setMenuOpen, navRef, menuButton } = useMenuDrawer()
  const pages = settingsGroupsFor(false, false, subscriptionNav(user)).flatMap((g) => g.pages)
  return (
    <div className="app-shell" data-section="settings">
      <div className={`sidebar-backdrop${menuOpen ? ' open' : ''}`} onClick={() => setMenuOpen(false)} />
      <nav id="main-menu" ref={navRef} className={`sidebar${menuOpen ? ' open' : ''}`} aria-label={tc('menu', 'Main')}>
        <div className="brand">
          <BrandMark />
          <span className="brand-text">
            <span className="brand-name">Mediarium</span>
          </span>
        </div>
        <div className="side-scroll">
          <div className="subnav" style={{ ['--nc' as string]: 'var(--c-settings)' }}>
            {pages.map((p) => (
              <NavLink key={p.to} to={p.to} style={{ ['--sc' as string]: p.color }} className={() => (pathname === p.to ? 'active' : '')}>
                <span className="nav-ico">
                  <Icon name={p.icon} size={15} />
                </span>
                <span className="nav-label">{p.label}</span>
              </NavLink>
            ))}
          </div>
        </div>
      </nav>
      <div className="main-area">
        <header className="topbar">
          <button ref={menuButton} className="menu-button" aria-controls="main-menu" onClick={() => setMenuOpen((o) => !o)} aria-label={t('Open menu')} aria-expanded={menuOpen}>
            <Icon name="menu" size={20} />
          </button>
          <p className="top-title">{t('Your account')}</p>
          <div className="top-actions">
            <span className="top-theme">
              <ThemeToggle />
            </span>
            <ProfileMenu />
          </div>
        </header>
        <main id="main" className="content" tabIndex={-1}>
          <Suspense fallback={<PageLoading />}>
            <Outlet />
          </Suspense>
        </main>
      </div>
    </div>
  )
}
