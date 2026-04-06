import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate } from 'react-router-dom'
import { useAuthStore } from '../../store/authStore'

const LANGUAGES = [
  { code: 'en', label: 'English' },
  { code: 'zh', label: '中文' },
  { code: 'ja', label: '日本語' },
] as const

export default function Navbar() {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const user = useAuthStore((s) => s.user)
  const logout = useAuthStore((s) => s.logout)
  const [langOpen, setLangOpen] = useState(false)
  const [userMenuOpen, setUserMenuOpen] = useState(false)

  const isEditor = user?.role === 'editor' || user?.role === 'admin'
  const isAdmin = user?.role === 'admin'

  function handleLangChange(code: string) {
    i18n.changeLanguage(code)
    localStorage.setItem('lang', code)
    setLangOpen(false)
  }

  function handleLogout() {
    logout()
    navigate('/login')
  }

  const currentLang = LANGUAGES.find((l) => l.code === i18n.language) ?? LANGUAGES[0]

  return (
    <nav className="flex h-14 items-center justify-between bg-gray-900 px-6 text-white">
      <div className="flex items-center gap-8">
        <Link to="/" className="text-lg font-bold tracking-tight">
          {t('app.title')}
        </Link>

        <div className="flex items-center gap-4">
          <Link
            to="/"
            className="rounded px-3 py-1.5 text-sm font-medium text-gray-300 transition hover:bg-gray-800 hover:text-white"
          >
            {t('nav.formulas')}
          </Link>

          {isEditor && (
            <Link
              to="/tables"
              className="rounded px-3 py-1.5 text-sm font-medium text-gray-300 transition hover:bg-gray-800 hover:text-white"
            >
              {t('nav.tables')}
            </Link>
          )}

          {isAdmin && (
            <Link
              to="/categories"
              className="rounded px-3 py-1.5 text-sm font-medium text-gray-300 transition hover:bg-gray-800 hover:text-white"
            >
              {t('nav.categories')}
            </Link>
          )}

          {isAdmin && (
            <Link
              to="/users"
              className="rounded px-3 py-1.5 text-sm font-medium text-gray-300 transition hover:bg-gray-800 hover:text-white"
            >
              {t('nav.users')}
            </Link>
          )}

          {isAdmin && (
            <Link
              to="/cache"
              className="rounded px-3 py-1.5 text-sm font-medium text-gray-300 transition hover:bg-gray-800 hover:text-white"
            >
              {t('nav.cache')}
            </Link>
          )}
        </div>
      </div>

      <div className="flex items-center gap-4">
        {/* Language switcher */}
        <div className="relative">
          <button
            onClick={() => setLangOpen(!langOpen)}
            className="rounded px-3 py-1.5 text-sm text-gray-300 transition hover:bg-gray-800 hover:text-white"
          >
            {currentLang.label}
          </button>
          {langOpen && (
            <div className="absolute right-0 top-full z-50 mt-1 w-32 rounded-lg bg-white py-1 shadow-lg">
              {LANGUAGES.map((lang) => (
                <button
                  key={lang.code}
                  onClick={() => handleLangChange(lang.code)}
                  className={`block w-full px-4 py-2 text-left text-sm transition hover:bg-gray-100 ${
                    lang.code === i18n.language
                      ? 'font-semibold text-indigo-600'
                      : 'text-gray-700'
                  }`}
                >
                  {lang.label}
                </button>
              ))}
            </div>
          )}
        </div>

        {/* User menu */}
        {user && (
          <div className="relative">
            <button
              onClick={() => setUserMenuOpen(!userMenuOpen)}
              className="flex items-center gap-2 rounded px-3 py-1.5 text-sm text-gray-300 transition hover:bg-gray-800 hover:text-white"
            >
              <span className="inline-flex h-7 w-7 items-center justify-center rounded-full bg-indigo-600 text-xs font-bold uppercase text-white">
                {user.username.charAt(0)}
              </span>
              <span>{user.username}</span>
              <span className="rounded bg-gray-700 px-1.5 py-0.5 text-xs text-gray-300">
                {user.role}
              </span>
            </button>
            {userMenuOpen && (
              <div className="absolute right-0 top-full z-50 mt-1 w-40 rounded-lg bg-white py-1 shadow-lg">
                <button
                  onClick={handleLogout}
                  className="block w-full px-4 py-2 text-left text-sm text-gray-700 transition hover:bg-gray-100"
                >
                  {t('nav.logout')}
                </button>
              </div>
            )}
          </div>
        )}
      </div>
    </nav>
  )
}
