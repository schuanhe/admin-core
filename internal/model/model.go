package model

import "encoding/json"

type Service struct {
	Pinned                bool            `json:"pinned"`
	NavigationOrder       int             `json:"navigation_order"`
	ID                    string          `json:"id"`
	Name                  string          `json:"name"`
	Version               string          `json:"version"`
	Description           string          `json:"description"`
	BaseURL               string          `json:"base_url"`
	ProtocolVersion       int             `json:"protocol_version"`
	HealthPath            string          `json:"health_path"`
	Activation            string          `json:"activation"`
	Online                string          `json:"online"`
	RegisteredAt          string          `json:"registered_at"`
	CheckedAt             *string         `json:"checked_at"`
	CreatedAt             string          `json:"created_at"`
	UpdatedAt             string          `json:"updated_at"`
	LastError             string          `json:"last_error"`
	LastRegistrationError string          `json:"last_registration_error"`
	Definition            json.RawMessage `json:"definition"`
}
type RegistrationEvent struct {
	ID        int64  `json:"id"`
	ServiceID string `json:"service_id"`
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	CreatedAt string `json:"created_at"`
}
type Audit struct {
	ID        int64  `json:"id"`
	Actor     string `json:"actor"`
	Action    string `json:"action"`
	Target    string `json:"target"`
	Message   string `json:"message"`
	CreatedAt string `json:"created_at"`
}
type Settings struct {
	HandbookEnabled       bool     `json:"handbook_enabled"`
	GatewayServiceCount   int      `json:"gateway_service_count"`
	SiteName              string   `json:"site_name"`
	CheckIntervalSeconds  int      `json:"check_interval_seconds"`
	AllowedServiceOrigins []string `json:"allowed_service_origins"`
	SessionHours          int      `json:"session_hours"`
}
