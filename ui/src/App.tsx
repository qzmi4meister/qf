import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { MantineProvider, createTheme } from '@mantine/core'
import { ModalsProvider } from '@mantine/modals'
import { Notifications } from '@mantine/notifications'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import '@mantine/core/styles.css'
import '@mantine/charts/styles.css'
import '@mantine/notifications/styles.css'

import RouteGuard from './components/RouteGuard'
import Layout from './components/Layout'
import Login from './pages/Login'
import Dashboard from './pages/Dashboard'
import Hosts from './pages/Hosts'
import HostDetail from './pages/HostDetail'
import Policies from './pages/Policies'
import PolicyDetail from './pages/PolicyDetail'
import ObjectGroups from './pages/ObjectGroups'
import DefaultPolicyPage from './pages/DefaultPolicy'
import Events from './pages/Events'
import Flows from './pages/Flows'
import AuditLog from './pages/AuditLog'
import Tokens from './pages/Tokens'
import Users from './pages/Users'

const theme = createTheme({
  colors: {
    dark: [
      '#cbd5e1', // [0] slate-300  primary text
      '#94a3b8', // [1] slate-400  dimmed text
      '#64748b', // [2] slate-500  placeholder
      '#475569', // [3] slate-600  borders
      '#334155', // [4] slate-700  subtle borders / hover
      '#1e293b', // [5] slate-800  input / element bg
      '#172033', // [6]            card / modal bg
      '#0f172a', // [7] slate-900  body bg
      '#0a1020', // [8]            AppShell nav bg
      '#020617', // [9] slate-950  deepest bg
    ],
  },
  primaryColor: 'indigo',
})

const qc = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
})

export default function App() {
  return (
    <QueryClientProvider client={qc}>
      <MantineProvider theme={theme} defaultColorScheme="auto">
        <ModalsProvider>
          <Notifications position="top-right" />
          <BrowserRouter basename="/app">
            <Routes>
              <Route path="/login" element={<Login />} />
              <Route
                path="/*"
                element={
                  <RouteGuard>
                    <Layout />
                  </RouteGuard>
                }
              >
                <Route index element={<Navigate to="/dashboard" replace />} />
                <Route path="dashboard" element={<Dashboard />} />
                <Route path="hosts" element={<Hosts />} />
                <Route path="hosts/:id" element={<HostDetail />} />
                <Route path="policies" element={<Policies />} />
                <Route path="policies/:id" element={<PolicyDetail />} />
                <Route path="object-groups" element={<ObjectGroups />} />
                <Route path="default-policy" element={<DefaultPolicyPage />} />
                <Route path="events" element={<Events />} />
                <Route path="flows" element={<Flows />} />
                <Route path="audit" element={<AuditLog />} />
                <Route path="tokens" element={<Tokens />} />
                <Route path="users" element={<Users />} />
              </Route>
            </Routes>
          </BrowserRouter>
        </ModalsProvider>
      </MantineProvider>
    </QueryClientProvider>
  )
}
