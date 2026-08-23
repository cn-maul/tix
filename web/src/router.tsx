import { lazy, Suspense } from 'react'
import { Routes, Route, Navigate } from 'react-router-dom'
import Layout from '@/components/Layout'
import RequireAuth from '@/components/RequireAuth'

const Dashboard = lazy(() => import('./pages/Dashboard'))
const TicketList = lazy(() => import('./pages/TicketList'))
const TicketDetail = lazy(() => import('./pages/TicketDetail'))
const TicketNew = lazy(() => import('./pages/TicketNew'))
const Submit = lazy(() => import('./pages/Submit'))
const Login = lazy(() => import('./pages/Login'))
const SettingsLayout = lazy(() => import('./pages/Settings/SettingsLayout'))
const General = lazy(() => import('./pages/Settings/General'))
const Notifications = lazy(() => import('./pages/Settings/Notifications'))
const Users = lazy(() => import('./pages/Settings/Users'))
const Categories = lazy(() => import('./pages/Settings/Categories'))
const Data = lazy(() => import('./pages/Settings/Data'))

export default function Router() {
  return (
    <Suspense
      fallback={
        <div className="flex min-h-screen items-center justify-center">
          <div className="size-8 animate-spin rounded-full border-2 border-muted border-t-primary" />
        </div>
      }
    >
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route element={<RequireAuth />}>
          <Route element={<Layout />}>
            <Route path="/" element={<TicketList status={0} />} />
            <Route path="/dashboard" element={<Dashboard />} />
            <Route path="/tickets" element={<TicketList status="" />} />
            <Route path="/tickets/done" element={<TicketList status={1} />} />
            <Route path="/tickets/new" element={<TicketNew />} />
            <Route path="/tickets/:id" element={<TicketDetail />} />
            <Route path="/settings" element={<SettingsLayout />}>
              <Route index element={<Navigate to="/settings/general" replace />} />
              <Route path="general" element={<General />} />
              <Route path="notifications" element={<Notifications />} />
              <Route path="users" element={<Users />} />
              <Route path="categories" element={<Categories />} />
              <Route path="data" element={<Data />} />
            </Route>
          </Route>
        </Route>
        <Route path="/submit" element={<Submit />} />
      </Routes>
    </Suspense>
  )
}