package store

import (
	"context"
	"database/sql"
	"errors"
)

type NavigationItem struct {
	ServiceID string `json:"service_id"`
	Pinned    bool   `json:"pinned"`
}

// 只更新管理员提交的服务，禁用服务的已有偏好不被菜单快照清除。
func (s *Store) UpdateNavigation(ctx context.Context, items []NavigationItem, actor string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	t := now()
	for index, item := range items {
		result, err := tx.ExecContext(ctx, "UPDATE services SET pinned=?,navigation_order=?,updated_at=? WHERE id=?", item.Pinned, index, t, item.ServiceID)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return ErrNotFound
		}
	}
	if err = audit(ctx, tx, actor, "navigation.update", "services", "调整服务导航顺序及固定状态"); err != nil {
		return err
	}
	return tx.Commit()
}

// 单独保存开关，保持旧版站点设置和后台周期任务的读取契约。
func (s *Store) HandbookEnabled(ctx context.Context) (bool, error) {
	var value string
	err := s.DB.QueryRowContext(ctx, "SELECT value FROM system_settings WHERE key='handbook_enabled'").Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return value == "true", err
}
