package main

// 示例的统计与进度绑定真实进程数据；组合页面不新增任何网关权限。
func workbenchPage() map[string]any {
	return map[string]any{
		"id": "workbench", "title": "工作台", "type": "compose", "source": map[string]any{"path": "/api/overview"},
		"class_name": "demo-workbench", "style": map[string]string{"padding": "4px"},
		"stylesheet": `.summary { padding: 20px; border: 1px solid var(--border-default); border-radius: 12px; background: var(--surface-raised); }
.primary-metric strong { color: var(--accent-text); }
.progress-panel { padding: 20px; background: var(--surface-sunken); border-radius: 12px; }
@media (max-width: 680px) { .summary { padding: 16px; } }`,
		"blocks": []any{
			map[string]any{"id": "summary", "type": "grid", "columns": 3, "gap": 24, "class_name": "summary", "children": []any{
				map[string]any{"id": "total", "type": "statistic", "title": "任务总数", "value_key": "task_count", "unit": "项", "class_name": "primary-metric"},
				map[string]any{"id": "archived", "type": "statistic", "title": "已归档", "value_key": "archived_count", "unit": "项"},
				map[string]any{"id": "owner", "type": "statistic", "title": "负责人", "value_key": "owner"},
			}},
			map[string]any{"id": "completion", "type": "progress", "title": "归档进度", "value_key": "completion", "class_name": "progress-panel"},
			map[string]any{"id": "work-tabs", "type": "tabs", "children": []any{
				map[string]any{"id": "tasks-tab", "type": "tab", "title": "任务", "children": []any{map[string]any{"id": "tasks-embed", "type": "page", "page_id": "tasks"}}},
				map[string]any{"id": "preferences-tab", "type": "tab", "title": "偏好", "children": []any{map[string]any{"id": "preferences-embed", "type": "page", "page_id": "preferences"}}},
				map[string]any{"id": "info-tab", "type": "tab", "title": "服务信息", "children": []any{
					map[string]any{"id": "info", "type": "descriptions", "columns": 2, "fields": []any{map[string]any{"key": "name", "label": "服务"}, map[string]any{"key": "version", "label": "版本"}, map[string]any{"key": "storage", "label": "数据存储"}, map[string]any{"key": "remark", "label": "备注"}}},
					map[string]any{"id": "divider", "type": "divider"},
					map[string]any{"id": "info-note", "type": "text", "text": "归档任务后，点击上方刷新可更新统计。"},
					map[string]any{"id": "info-list", "type": "list", "items": []string{"任务与偏好由本服务保存。", "演示数据会在服务重启后重置。"}},
					map[string]any{"id": "preferences-link", "type": "page-link", "title": "打开偏好设置", "page_id": "preferences"},
				}},
			}},
		},
	}
}
