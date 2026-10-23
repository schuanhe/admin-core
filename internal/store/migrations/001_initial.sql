CREATE TABLE administrators (
 id INTEGER PRIMARY KEY CHECK(id=1),
 username TEXT NOT NULL UNIQUE,
 password_hash TEXT NOT NULL,
 created_at TEXT NOT NULL,
 updated_at TEXT NOT NULL
);
CREATE TABLE sessions (
 token_hash TEXT PRIMARY KEY,
 administrator_id INTEGER NOT NULL REFERENCES administrators(id) ON DELETE CASCADE,
 csrf_token TEXT NOT NULL,
 expires_at TEXT NOT NULL,
 created_at TEXT NOT NULL
);
CREATE INDEX sessions_expiration ON sessions(expires_at);
CREATE TABLE services (
 id TEXT PRIMARY KEY,
 name TEXT NOT NULL,
 version TEXT NOT NULL,
 description TEXT NOT NULL DEFAULT '',
 base_url TEXT NOT NULL,
 protocol_version INTEGER NOT NULL CHECK(protocol_version=1),
 health_path TEXT NOT NULL,
 definition TEXT NOT NULL,
 activation TEXT NOT NULL CHECK(activation IN ('pending','enabled','disabled')),
 online TEXT NOT NULL CHECK(online IN ('unknown','online','offline')),
 registered_at TEXT NOT NULL,
 checked_at TEXT,
 created_at TEXT NOT NULL,
 updated_at TEXT NOT NULL,
 last_error TEXT NOT NULL DEFAULT '',
 last_registration_error TEXT NOT NULL DEFAULT ''
);
CREATE INDEX services_activation ON services(activation);
CREATE INDEX services_online ON services(online);
CREATE INDEX services_registered ON services(registered_at DESC);
CREATE TABLE registration_events (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 service_id TEXT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
 success INTEGER NOT NULL CHECK(success IN (0,1)),
 message TEXT NOT NULL,
 created_at TEXT NOT NULL
);
CREATE INDEX registration_events_service ON registration_events(service_id,id DESC);
CREATE TABLE audit_logs (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 actor TEXT NOT NULL,
 action TEXT NOT NULL,
 target TEXT NOT NULL,
 message TEXT NOT NULL,
 created_at TEXT NOT NULL
);
CREATE INDEX audit_logs_time ON audit_logs(id DESC);
CREATE TABLE system_settings (
 key TEXT PRIMARY KEY,
 value TEXT NOT NULL,
 created_at TEXT NOT NULL,
 updated_at TEXT NOT NULL
);
