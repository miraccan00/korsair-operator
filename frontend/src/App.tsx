import { BrowserRouter, Routes, Route } from 'react-router-dom'
import { Navbar } from './components/Navbar'
import { Dashboard } from './pages/Dashboard'
import { ConfigsPage } from './pages/ConfigsPage'
import { ImagesPage } from './pages/ImagesPage'
import { JobsPage } from './pages/JobsPage'
import { JobDetailPage } from './pages/JobDetailPage'
import { ClustersPage } from './pages/ClustersPage'

export default function App() {
  return (
    <BrowserRouter>
      <div className="flex min-h-screen bg-gray-50">
        <Navbar />
        <main className="flex-1 overflow-auto min-w-0">
          <Routes>
            <Route path="/"                          element={<Dashboard />} />
            <Route path="/configs"                   element={<ConfigsPage />} />
            <Route path="/images"                    element={<ImagesPage />} />
            <Route path="/jobs"                      element={<JobsPage />} />
            <Route path="/jobs/:namespace/:name"     element={<JobDetailPage />} />
            <Route path="/clusters"                  element={<ClustersPage />} />
          </Routes>
        </main>
      </div>
    </BrowserRouter>
  )
}
