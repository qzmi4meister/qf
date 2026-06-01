import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { MantineProvider } from '@mantine/core'
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
      <MantineProvider defaultColorScheme="auto">
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
