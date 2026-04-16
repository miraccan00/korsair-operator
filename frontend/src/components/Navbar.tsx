import { NavLink } from 'react-router-dom'

interface NavItem {
  to: string
  label: string
  icon: string
}

const navItems: NavItem[] = [
  { to: '/',         label: 'Dashboard', icon: '🛡️' },
  { to: '/configs',  label: 'Configs',   icon: '⚙️' },
  { to: '/images',   label: 'Images',    icon: '📦' },
  { to: '/jobs',     label: 'Scan Jobs', icon: '🔍' },
  { to: '/clusters', label: 'Clusters',  icon: '🌐' },
]

export function Navbar() {
  return (
    <nav className="w-56 min-h-screen bg-gray-900 text-gray-100 flex flex-col shadow-xl">
      {/* Logo / Brand */}
      <div className="px-5 py-6 border-b border-gray-700">
        <div className="flex items-center gap-2">
          <span className="text-2xl">🛡️</span>
          <div>
            <p className="text-sm font-bold leading-tight text-white">Korsair</p>
            <p className="text-xs text-gray-400 leading-tight">Vulnerability Scanner</p>
          </div>
        </div>
      </div>

      {/* Navigation links */}
      <div className="flex-1 px-3 py-4 space-y-1">
        {navItems.map(({ to, label, icon }) => (
          <NavLink
            key={to}
            to={to}
            end={to === '/'}
            className={({ isActive }) =>
              `flex items-center gap-3 px-3 py-2 rounded-lg text-sm font-medium transition-colors ${
                isActive
                  ? 'bg-blue-600 text-white'
                  : 'text-gray-400 hover:bg-gray-800 hover:text-white'
              }`
            }
          >
            <span>{icon}</span>
            {label}
          </NavLink>
        ))}
      </div>

      {/* Footer */}
      <div className="px-5 py-4 border-t border-gray-700">
        <p className="text-xs text-gray-500">v0.4.0</p>
      </div>
    </nav>
  )
}
