import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { MantineProvider, createTheme } from '@mantine/core'
import { ModalsProvider } from '@mantine/modals'
import { Notifications } from '@mantine/notifications'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import '@mantine/core/styles.css'
import '@mantine/charts/styles.css'
import '@mantine/notifications/styles.css'
import './qf-tokens.css'

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
      '#eaeef3', // [0] slate-100  primary text
      '#d4dae3', // [1] slate-200
      '#aeb8c7', // [2] slate-300
      '#818da0', // [3] slate-400
      '#5d6878', // [4] slate-500
      '#2a313d', // [5] slate-700  inputs / elements
      '#141821', // [6] slate-850  card / modal bg
      '#090c12', // [7] slate-950  body bg
      '#060810', // [8]            deepest
      '#030508', // [9]
    ],
    indigo: [
      '#eef1ff', // [0]
      '#e0e4ff', // [1]
      '#c5ccff', // [2]
      '#a1abfb', // [3]
      '#7e88f5', // [4] dark-mode primary
      '#6366e8', // [5] brand anchor
      '#5052d6', // [6] light-mode primary
      '#4144b4', // [7]
      '#363891', // [8]
      '#1a1b45', // [9]
    ],
  },
  primaryColor: 'indigo',
  fontFamily: '"Inter", ui-sans-serif, system-ui, -apple-system, sans-serif',
  fontFamilyMonospace: 'ui-monospace, "JetBrains Mono", "SFMono-Regular", Menlo, Consolas, monospace',
  fontSizes: { xs: '11px', sm: '12px', md: '13px', lg: '15px', xl: '18px' },
  radius: { xs: '4px', sm: '6px', md: '8px', lg: '10px', xl: '14px' },
  defaultRadius: 'md',
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
