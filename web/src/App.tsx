import { Routes, Route, Navigate, useParams } from 'react-router-dom'
import { getProject } from './api/client.ts'
import { useApi } from './hooks/useApi.ts'
import { ProjectHeader } from './components/ProjectHeader.tsx'
import { ModeSwitcher } from './components/ModeSwitcher.tsx'
import { LoadingState } from './components/LoadingState.tsx'
import { ErrorState } from './components/ErrorState.tsx'
import { FilesPage } from './pages/FilesPage.tsx'
import { FunctionsPage } from './pages/FunctionsPage.tsx'
import { TestsPage } from './pages/TestsPage.tsx'
import { DiffPage } from './pages/DiffPage.tsx'

function ProjectLayout() {
  const { projectId } = useParams<{ projectId: string }>()
  const { data: project, loading, error } = useApi(() => getProject(projectId!), [projectId])

  if (loading) return <LoadingState message="Loading project…" />
  if (error) return <ErrorState error={error} />

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100vh' }}>
      <ProjectHeader project={project ?? undefined} />
      <div style={{ display: 'flex', flex: 1, overflow: 'hidden' }}>
        <ModeSwitcher projectId={projectId!} />
        <main style={{ flex: 1, overflow: 'auto' }}>
          <Routes>
            <Route path="files/*" element={<FilesPage />} />
            <Route path="functions" element={<FunctionsPage />} />
            <Route path="functions/:symbolId" element={<FunctionsPage />} />
            <Route path="tests" element={<TestsPage />} />
            <Route path="tests/:testId" element={<TestsPage />} />
            <Route path="tests/:testId/golden/:caseId/diff" element={<DiffPage />} />
            <Route path="*" element={<Navigate to="files" replace />} />
          </Routes>
        </main>
      </div>
    </div>
  )
}

function App() {
  return (
    <Routes>
      <Route path="/projects/:projectId/*" element={<ProjectLayout />} />
      <Route path="*" element={<Navigate to="/projects/default" replace />} />
    </Routes>
  )
}

export default App
