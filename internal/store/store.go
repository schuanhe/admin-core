package store

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
	"workstation/internal/model"
	"workstation/internal/protocol"
)

//go:embed migrations/*.sql
var migrations embed.FS
var ErrNotFound = errors.New("记录不存在")
var ErrInitialized = errors.New("管理员已初始化")

type Store struct{ DB *sql.DB }

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func Open(path string) (*Store, error) {
	if path != ":memory:" {
		if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
			return nil, e
		}
	}
	db, e := sql.Open("sqlite", path)
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	s := &Store{DB: db}
	for _, q := range []string{"PRAGMA foreign_keys=ON", "PRAGMA busy_timeout=5000", "PRAGMA journal_mode=WAL"} {
		if _, e = db.Exec(q); e != nil {
			db.Close()
			return nil, e
		}
	}
	if e = s.migrate(); e != nil {
		db.Close()
		return nil, e
	}
	return s, nil
}
func (s *Store) Close() error { return s.DB.Close() }
func (s *Store) migrate() error {
	if _, e := s.DB.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TEXT NOT NULL)`); e != nil {
		return e
	}
	entries, e := migrations.ReadDir("migrations")
	if e != nil {
		return e
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		var n int
		if e = s.DB.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version=?", entry.Name()).Scan(&n); e != nil {
			return e
		}
		if n > 0 {
			continue
		}
		b, e := migrations.ReadFile("migrations/" + entry.Name())
		if e != nil {
			return e
		}
		tx, e := s.DB.Begin()
		if e != nil {
			return e
		}
		if _, e = tx.Exec(string(b)); e == nil {
			_, e = tx.Exec("INSERT INTO schema_migrations VALUES (?,?)", entry.Name(), now())
		}
		if e != nil {
			tx.Rollback()
			return fmt.Errorf("数据库迁移失败: %w", e)
		}
		if e = tx.Commit(); e != nil {
			return e
		}
	}
	return nil
}
func audit(ctx context.Context, tx *sql.Tx, actor, action, target, message string) error {
	_, e := tx.ExecContext(ctx, "INSERT INTO audit_logs(actor,action,target,message,created_at) VALUES(?,?,?,?,?)", actor, action, target, message, now())
	return e
}
func (s *Store) Audit(ctx context.Context, actor, action, target, message string) error {
	_, e := s.DB.ExecContext(ctx, "INSERT INTO audit_logs(actor,action,target,message,created_at) VALUES(?,?,?,?,?)", actor, action, target, message, now())
	return e
}
func (s *Store) Initialized(ctx context.Context) (bool, error) {
	var n int
	e := s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM administrators").Scan(&n)
	return n > 0, e
}
func (s *Store) Setup(ctx context.Context, username, hash string) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var n int
	if e = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM administrators").Scan(&n); e != nil {
		return e
	}
	if n > 0 {
		return ErrInitialized
	}
	t := now()
	if _, e = tx.ExecContext(ctx, "INSERT INTO administrators VALUES(1,?,?,?,?)", username, hash, t, t); e != nil {
		return e
	}
	if e = audit(ctx, tx, username, "auth.setup", "administrator", "初始化管理员"); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) Password(ctx context.Context, username string) (string, error) {
	var h string
	e := s.DB.QueryRowContext(ctx, "SELECT password_hash FROM administrators WHERE username=?", username).Scan(&h)
	if errors.Is(e, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return h, e
}
func (s *Store) CreateSession(ctx context.Context, hash, csrf, username string, expires time.Time) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at<=?", now()); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "INSERT INTO sessions VALUES(?,1,?,?,?)", hash, csrf, expires.UTC().Format(time.RFC3339Nano), now()); e != nil {
		return e
	}
	if e = audit(ctx, tx, username, "auth.login", "administrator", "管理员登录"); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) Session(ctx context.Context, hash string) (username, csrf string, err error) {
	err = s.DB.QueryRowContext(ctx, "SELECT a.username,s.csrf_token FROM sessions s JOIN administrators a ON a.id=s.administrator_id WHERE s.token_hash=? AND s.expires_at>?", hash, now()).Scan(&username, &csrf)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return
}
func (s *Store) DeleteSession(ctx context.Context, hash, username string) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash=?", hash); e != nil {
		return e
	}
	if e = audit(ctx, tx, username, "auth.logout", "administrator", "管理员退出"); e != nil {
		return e
	}
	return tx.Commit()
}

const registrationColumns = "id,name,version,description,base_url,protocol_version,health_path,definition,activation,online,registered_at,checked_at,created_at,updated_at,last_error,last_registration_error"
const serviceColumns = registrationColumns + ",pinned,navigation_order"

type scanner interface{ Scan(...any) error }

func scanService(row scanner) (model.Service, error) {
	var s model.Service
	var definition string
	e := row.Scan(&s.ID, &s.Name, &s.Version, &s.Description, &s.BaseURL, &s.ProtocolVersion, &s.HealthPath, &definition, &s.Activation, &s.Online, &s.RegisteredAt, &s.CheckedAt, &s.CreatedAt, &s.UpdatedAt, &s.LastError, &s.LastRegistrationError, &s.Pinned, &s.NavigationOrder)
	s.Definition = json.RawMessage(definition)
	if errors.Is(e, sql.ErrNoRows) {
		e = ErrNotFound
	}
	return s, e
}
func (s *Store) Service(ctx context.Context, id string) (model.Service, error) {
	return scanService(s.DB.QueryRowContext(ctx, "SELECT "+serviceColumns+" FROM services WHERE id=?", id))
}
func (s *Store) Services(ctx context.Context, q, activation, online string) ([]model.Service, error) {
	query := "SELECT " + serviceColumns + " FROM services WHERE 1=1"
	args := []any{}
	if q != "" {
		query += " AND (instr(lower(name),lower(?))>0 OR instr(lower(id),lower(?))>0 OR instr(lower(base_url),lower(?))>0)"
		args = append(args, q, q, q)
	}
	if activation != "" {
		query += " AND activation=?"
		args = append(args, activation)
	}
	if online != "" {
		query += " AND online=?"
		args = append(args, online)
	}
	query += " ORDER BY registered_at DESC,id"
	rows, e := s.DB.QueryContext(ctx, query, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	items := []model.Service{}
	for rows.Next() {
		v, e := scanService(rows)
		if e != nil {
			return nil, e
		}
		items = append(items, v)
	}
	return items, rows.Err()
}
func (s *Store) Register(ctx context.Context, r protocol.Registration) (model.Service, error) {
	b, e := json.Marshal(r.Definition)
	if e != nil {
		return model.Service{}, e
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return model.Service{}, e
	}
	defer tx.Rollback()
	t := now()
	_, e = tx.ExecContext(ctx, `INSERT INTO services (`+registrationColumns+`) VALUES(?,?,?,?,?,?,?,?,'pending','unknown',?,NULL,?,?,'','') ON CONFLICT(id) DO UPDATE SET name=excluded.name,version=excluded.version,description=excluded.description,base_url=excluded.base_url,protocol_version=excluded.protocol_version,health_path=excluded.health_path,definition=excluded.definition,registered_at=excluded.registered_at,updated_at=excluded.updated_at,last_registration_error='',online=CASE WHEN services.base_url<>excluded.base_url OR services.health_path<>excluded.health_path THEN 'unknown' ELSE services.online END,checked_at=CASE WHEN services.base_url<>excluded.base_url OR services.health_path<>excluded.health_path THEN NULL ELSE services.checked_at END,last_error=CASE WHEN services.base_url<>excluded.base_url OR services.health_path<>excluded.health_path THEN '' ELSE services.last_error END`, r.ID, r.Name, r.Version, r.Description, r.BaseURL, r.ProtocolVersion, r.HealthPath, string(b), t, t, t)
	if e != nil {
		return model.Service{}, e
	}
	if _, e = tx.ExecContext(ctx, "INSERT INTO registration_events(service_id,success,message,created_at) VALUES(?,1,?,?)", r.ID, "登记信息已更新，管理员启用状态保持不变", t); e != nil {
		return model.Service{}, e
	}
	if e = audit(ctx, tx, "service:"+r.ID, "service.register", r.ID, "服务注册成功"); e != nil {
		return model.Service{}, e
	}
	if e = tx.Commit(); e != nil {
		return model.Service{}, e
	}
	return s.Service(ctx, r.ID)
}
func (s *Store) RegistrationFailure(ctx context.Context, id, message string) error {
	if !protocol.ValidServiceID(id) {
		id = "unknown"
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	t := now()
	res, e := tx.ExecContext(ctx, "UPDATE services SET last_registration_error=?,updated_at=? WHERE id=?", message, t, id)
	if e != nil {
		return e
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		if _, e = tx.ExecContext(ctx, "INSERT INTO registration_events(service_id,success,message,created_at) VALUES(?,0,?,?)", id, message, t); e != nil {
			return e
		}
	}
	if e = audit(ctx, tx, "registry", "service.register_failed", id, message); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) Events(ctx context.Context, id string) ([]model.RegistrationEvent, error) {
	rows, e := s.DB.QueryContext(ctx, "SELECT id,service_id,success,message,created_at FROM registration_events WHERE service_id=? ORDER BY id DESC LIMIT 30", id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []model.RegistrationEvent{}
	for rows.Next() {
		var x model.RegistrationEvent
		if e = rows.Scan(&x.ID, &x.ServiceID, &x.Success, &x.Message, &x.CreatedAt); e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (s *Store) SetActivation(ctx context.Context, id, state, actor string) (model.Service, error) {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return model.Service{}, e
	}
	defer tx.Rollback()
	res, e := tx.ExecContext(ctx, "UPDATE services SET activation=?,updated_at=? WHERE id=?", state, now(), id)
	if e != nil {
		return model.Service{}, e
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return model.Service{}, ErrNotFound
	}
	if e = audit(ctx, tx, actor, "service."+state, id, "更新服务启用状态"); e != nil {
		return model.Service{}, e
	}
	if e = tx.Commit(); e != nil {
		return model.Service{}, e
	}
	return s.Service(ctx, id)
}
func (s *Store) DeleteService(ctx context.Context, id, actor string) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	res, e := tx.ExecContext(ctx, "DELETE FROM services WHERE id=?", id)
	if e != nil {
		return e
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	if e = audit(ctx, tx, actor, "service.delete", id, "仅删除中心登记记录和关联注册事件"); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) SetHealth(ctx context.Context, v model.Service, online, message string) error {
	_, e := s.DB.ExecContext(ctx, "UPDATE services SET online=?,last_error=?,checked_at=?,updated_at=? WHERE id=? AND registered_at=?", online, message, now(), now(), v.ID, v.RegisteredAt)
	return e
}
func (s *Store) Settings(ctx context.Context, defaultInterval int) (string, int, error) {
	var raw string
	e := s.DB.QueryRowContext(ctx, "SELECT value FROM system_settings WHERE key='general'").Scan(&raw)
	if errors.Is(e, sql.ErrNoRows) {
		return "个人工作站", defaultInterval, nil
	}
	if e != nil {
		return "", 0, e
	}
	var v struct {
		SiteName string `json:"site_name"`
		Interval int    `json:"check_interval_seconds"`
	}
	if e = json.Unmarshal([]byte(raw), &v); e != nil {
		return "", 0, e
	}
	return v.SiteName, v.Interval, nil
}
func (s *Store) UpdateSettings(ctx context.Context, name string, interval int, actor string, handbookEnabled ...bool) error {
	b, _ := json.Marshal(map[string]any{"site_name": name, "check_interval_seconds": interval})
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	t := now()
	if _, e = tx.ExecContext(ctx, "INSERT INTO system_settings VALUES('general',?,?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at", string(b), t, t); e != nil {
		return e
	}
	if len(handbookEnabled) > 0 {
		value := "false"
		if handbookEnabled[0] {
			value = "true"
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO system_settings VALUES('handbook_enabled',?,?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at", value, t, t); e != nil {
			return e
		}
		if e = audit(ctx, tx, actor, "handbook.visibility", "integration", "接入手册公开状态: "+value); e != nil {
			return e
		}
	}
	if e = audit(ctx, tx, actor, "settings.update", "general", "更新站点名称与检测间隔"); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) Audits(ctx context.Context, page, size int) ([]model.Audit, int, error) {
	var total int
	if e := s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_logs").Scan(&total); e != nil {
		return nil, 0, e
	}
	rows, e := s.DB.QueryContext(ctx, "SELECT id,actor,action,target,message,created_at FROM audit_logs ORDER BY id DESC LIMIT ? OFFSET ?", size, (page-1)*size)
	if e != nil {
		return nil, 0, e
	}
	defer rows.Close()
	out := []model.Audit{}
	for rows.Next() {
		var v model.Audit
		if e = rows.Scan(&v.ID, &v.Actor, &v.Action, &v.Target, &v.Message, &v.CreatedAt); e != nil {
			return nil, 0, e
		}
		out = append(out, v)
	}
	return out, total, rows.Err()
}

// Prune 保留最近审计及每个服务的注册事件，避免长期运行无限积累。
func (s *Store) Prune(ctx context.Context) error {
	for _, q := range []string{"DELETE FROM sessions WHERE expires_at <= ?", "DELETE FROM audit_logs WHERE id NOT IN (SELECT id FROM audit_logs ORDER BY id DESC LIMIT 10000)", "DELETE FROM registration_events WHERE id NOT IN (SELECT id FROM (SELECT id,row_number() OVER(PARTITION BY service_id ORDER BY id DESC) AS rn FROM registration_events) WHERE rn<=100)"} {
		var e error
		if strings.Contains(q, "?") {
			_, e = s.DB.ExecContext(ctx, q, now())
		} else {
			_, e = s.DB.ExecContext(ctx, q)
		}
		if e != nil {
			return e
		}
	}
	return nil
}
