package model

import "fmt"

// IsSQLite 检测当前数据库是否为 SQLite
func IsSQLite() bool {
	if DB == nil {
		return false
	}
	return DB.Dialector.Name() == "sqlite"
}

// RandomOrder 返回随机排序的 SQL 函数
// MySQL: RAND()  SQLite: RANDOM()
func RandomOrder() string {
	if IsSQLite() {
		return "RANDOM()"
	}
	return "RAND()"
}

// NowExpr 返回当前时间的 SQL 表达式
// MySQL: NOW()  SQLite: datetime('now','localtime')
func NowExpr() string {
	if IsSQLite() {
		return "datetime('now','localtime')"
	}
	return "NOW()"
}

// DateFormat 返回日期格式化的 SQL 表达式
// MySQL: DATE_FORMAT(column, format)
// SQLite: strftime(format, column)
// format 使用 MySQL 风格 (如 %H:00, %m-%d, %Y-%m-%d)，自动转换为 SQLite 兼容格式
func DateFormat(column, format string) string {
	if IsSQLite() {
		return fmt.Sprintf("strftime('%s', %s)", format, column)
	}
	return fmt.Sprintf("DATE_FORMAT(%s, '%s')", column, format)
}

// DateFormatParam 返回日期格式化的 SQL 表达式（格式作为参数传入）
// 用于替代 DATE_FORMAT(created_at, ?) 的场景
// MySQL: DATE_FORMAT(column, ?)   SQLite: strftime(?, column)
func DateFormatParam(column string) string {
	if IsSQLite() {
		return fmt.Sprintf("strftime(?, %s)", column)
	}
	return fmt.Sprintf("DATE_FORMAT(%s, ?)", column)
}

// DateGroupBy 返回按时间分组的 SQL 表达式
// period: "hour", "date", "yearweek"
func DateGroupBy(column, period string) string {
	if IsSQLite() {
		switch period {
		case "hour":
			return fmt.Sprintf("strftime('%%H', %s)", column)
		case "date":
			return fmt.Sprintf("date(%s)", column)
		case "yearweek":
			return fmt.Sprintf("strftime('%%Y-%%W', %s)", column)
		}
		return fmt.Sprintf("date(%s)", column)
	}

	switch period {
	case "hour":
		return fmt.Sprintf("HOUR(%s)", column)
	case "date":
		return fmt.Sprintf("DATE(%s)", column)
	case "yearweek":
		return fmt.Sprintf("YEARWEEK(%s, 1)", column)
	}
	return fmt.Sprintf("DATE(%s)", column)
}
