export type Activation = 'pending' | 'enabled' | 'disabled'
export type Online = 'unknown' | 'online' | 'offline'
export interface Menu {
  id: string
  label: string
  page_id: string
}
export interface Definition {
  schema_version: number
  menus: Menu[]
  pages: unknown[]
}
export interface Service {
  pinned: boolean
  navigation_order: number
  id: string
  name: string
  version: string
  description: string
  base_url: string
  protocol_version: number
  health_path: string
  activation: Activation
  online: Online
  registered_at: string
  checked_at: string | null
  created_at: string
  updated_at: string
  last_error: string
  last_registration_error: string
  definition: Definition
}
export interface RegistrationEvent {
  id: number
  service_id: string
  success: boolean
  message: string
  created_at: string
}
export interface Dashboard {
  total: number
  enabled: number
  offline: number
  pending: number
  recent: Service[]
}
export interface Settings {
  handbook_enabled: boolean
  gateway_service_count: number
  site_name: string
  check_interval_seconds: number
  allowed_service_origins: string[]
  session_hours: number
}
export interface Audit {
  id: number
  actor: string
  action: string
  target: string
  message: string
  created_at: string
}
export interface Session {
  user: { username: string }
  csrf_token: string
}
