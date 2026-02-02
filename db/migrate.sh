#!/bin/bash
# ========================================
# EzPay 数据库版本管理工具
# 自动检测版本并应用增量迁移
# ========================================

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MIGRATIONS_DIR="$SCRIPT_DIR/migrations"

# 数据库配置
DB_HOST="172.16.1.10"
DB_USER="ezpay"
DB_PASS="Admin3579"
DB_NAME="ezpay"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# MySQL 命令简化
mysql_exec() {
    mysql -h "$DB_HOST" -u "$DB_USER" -p"$DB_PASS" --skip-ssl "$DB_NAME" -N -s "$@" 2>&1 | grep -v "Deprecated"
}

mysql_exec_file() {
    mysql -h "$DB_HOST" -u "$DB_USER" -p"$DB_PASS" --skip-ssl "$DB_NAME" < "$1" 2>&1 | grep -v "Deprecated"
}

# ========================================
# 函数定义
# ========================================

print_header() {
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}========================================${NC}"
}

print_step() {
    echo -e "${CYAN}>>> $1${NC}"
}

print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠ $1${NC}"
}

print_error() {
    echo -e "${RED}✗ $1${NC}"
}

# 初始化版本管理表
init_migrations_table() {
    print_step "检查版本管理表..."

    local table_exists=$(mysql_exec -e "SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = '$DB_NAME' AND TABLE_NAME = 'database_migrations'")

    if [ "$table_exists" = "0" ]; then
        print_warning "版本管理表不存在，正在创建..."

        mysql_exec <<-EOF
CREATE TABLE IF NOT EXISTS \`database_migrations\` (
  \`id\` bigint unsigned NOT NULL AUTO_INCREMENT,
  \`version\` varchar(50) NOT NULL COMMENT '版本号',
  \`description\` varchar(200) NOT NULL COMMENT '迁移描述',
  \`script_name\` varchar(100) NOT NULL COMMENT '脚本文件名',
  \`checksum\` varchar(64) DEFAULT NULL COMMENT 'SHA256校验和',
  \`executed_at\` datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '执行时间',
  \`execution_time\` int DEFAULT 0 COMMENT '执行耗时（毫秒）',
  \`status\` enum('SUCCESS','FAILED','ROLLBACK') NOT NULL DEFAULT 'SUCCESS' COMMENT '执行状态',
  \`error_message\` text COMMENT '错误信息',
  PRIMARY KEY (\`id\`),
  UNIQUE KEY \`idx_version\` (\`version\`),
  KEY \`idx_executed_at\` (\`executed_at\`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='数据库迁移版本管理表';
EOF

        print_success "版本管理表创建成功"
    else
        print_success "版本管理表已存在"
    fi
}

# 获取当前数据库版本
get_current_version() {
    local version=$(mysql_exec -e "SELECT COALESCE(MAX(version), 'V000') FROM database_migrations WHERE status = 'SUCCESS'")
    echo "$version"
}

# 获取已应用的迁移列表
get_applied_migrations() {
    mysql_exec -e "SELECT version FROM database_migrations WHERE status = 'SUCCESS' ORDER BY version"
}

# 扫描待执行的迁移文件
scan_pending_migrations() {
    local current_version="$1"
    local pending=()

    if [ ! -d "$MIGRATIONS_DIR" ]; then
        print_error "迁移目录不存在: $MIGRATIONS_DIR"
        return 1
    fi

    # 扫描所有迁移文件
    for file in "$MIGRATIONS_DIR"/V[0-9]*__*.sql; do
        [ -f "$file" ] || continue

        local filename=$(basename "$file")
        local version=$(echo "$filename" | grep -oP '^V\d+')

        # 检查是否已应用
        local is_applied=$(mysql_exec -e "SELECT COUNT(*) FROM database_migrations WHERE version = '$version' AND status = 'SUCCESS'")

        if [ "$is_applied" = "0" ]; then
            pending+=("$filename")
        fi
    done

    # 排序
    IFS=$'\n' sorted=($(sort <<<"${pending[*]}"))
    unset IFS

    echo "${sorted[@]}"
}

# 执行单个迁移
execute_migration() {
    local file="$1"
    local filename=$(basename "$file")
    local version=$(echo "$filename" | grep -oP '^V\d+')
    local description=$(echo "$filename" | sed -E 's/^V[0-9]+__(.+)\.sql$/\1/' | tr '_' ' ')

    print_step "执行迁移: $filename"
    echo -e "  版本: ${CYAN}$version${NC}"
    echo -e "  描述: $description"

    # 计算校验和
    local checksum=$(sha256sum "$file" | awk '{print $1}')

    # 开始计时
    local start_time=$(date +%s%3N)

    # 执行迁移
    local error_msg=""
    if mysql_exec_file "$file" > /tmp/migration_output.log 2>&1; then
        local end_time=$(date +%s%3N)
        local execution_time=$((end_time - start_time))

        # 记录成功
        mysql_exec <<-EOF
INSERT INTO database_migrations (version, description, script_name, checksum, execution_time, status)
VALUES ('$version', '$description', '$filename', '$checksum', $execution_time, 'SUCCESS');
EOF

        print_success "迁移成功 (耗时: ${execution_time}ms)"

        # 显示迁移输出
        if [ -s /tmp/migration_output.log ]; then
            echo -e "${BLUE}输出:${NC}"
            cat /tmp/migration_output.log | grep -v "Deprecated" | sed 's/^/  /'
        fi

        return 0
    else
        local end_time=$(date +%s%3N)
        local execution_time=$((end_time - start_time))
        error_msg=$(cat /tmp/migration_output.log | grep -v "Deprecated")

        # 记录失败
        mysql_exec <<-EOF
INSERT INTO database_migrations (version, description, script_name, checksum, execution_time, status, error_message)
VALUES ('$version', '$description', '$filename', '$checksum', $execution_time, 'FAILED', $(echo "$error_msg" | sed "s/'/''/g" | awk '{printf "\"%s\"", $0}'));
EOF

        print_error "迁移失败"
        echo -e "${RED}错误信息:${NC}"
        echo "$error_msg" | sed 's/^/  /'

        return 1
    fi
}

# 显示迁移历史
show_history() {
    print_header "迁移历史"

    mysql_exec -e "SELECT version, description, executed_at, execution_time, status FROM database_migrations ORDER BY version" | \
        awk 'BEGIN {
            printf "%-10s %-30s %-25s %-12s %-10s\n", "版本", "描述", "执行时间", "耗时(ms)", "状态"
            printf "%-10s %-30s %-25s %-12s %-10s\n", "----------", "------------------------------", "-------------------------", "------------", "----------"
        }
        {
            printf "%-10s %-30s %-25s %-12s %-10s\n", $1, $2, $3, $4, $5
        }'

    echo ""
}

# 显示当前状态
show_status() {
    print_header "数据库迁移状态"

    local current_version=$(get_current_version)
    local total_applied=$(mysql_exec -e "SELECT COUNT(*) FROM database_migrations WHERE status = 'SUCCESS'")
    local total_failed=$(mysql_exec -e "SELECT COUNT(*) FROM database_migrations WHERE status = 'FAILED'")

    echo -e "${CYAN}当前版本:${NC} $current_version"
    echo -e "${CYAN}已应用迁移:${NC} $total_applied"
    if [ "$total_failed" != "0" ]; then
        echo -e "${RED}失败迁移:${NC} $total_failed"
    fi
    echo ""

    # 检查待执行的迁移
    local pending=($(scan_pending_migrations "$current_version"))

    if [ ${#pending[@]} -gt 0 ]; then
        echo -e "${YELLOW}待执行迁移 (${#pending[@]}):${NC}"
        for migration in "${pending[@]}"; do
            local version=$(echo "$migration" | grep -oP '^V\d+')
            local desc=$(echo "$migration" | sed -E 's/^V[0-9]+__(.+)\.sql$/\1/' | tr '_' ' ')
            echo -e "  ${CYAN}$version${NC} - $desc"
        done
    else
        echo -e "${GREEN}✓ 数据库已是最新版本${NC}"
    fi

    echo ""
}

# 备份数据库
backup_database() {
    local backup_file="backup_$(date +%Y%m%d_%H%M%S).sql"

    print_step "备份数据库到: $backup_file"

    if mysqldump -h "$DB_HOST" -u "$DB_USER" -p"$DB_PASS" --skip-ssl "$DB_NAME" > "$backup_file" 2>/dev/null; then
        local size=$(ls -lh "$backup_file" | awk '{print $5}')
        print_success "备份成功 ($size)"
        echo "$backup_file"
    else
        print_error "备份失败"
        return 1
    fi
}

# 主函数：执行迁移
run_migrations() {
    print_header "EzPay 数据库迁移工具"
    echo ""

    # 初始化
    init_migrations_table
    echo ""

    # 显示当前状态
    show_status

    # 扫描待执行的迁移
    local current_version=$(get_current_version)
    local pending=($(scan_pending_migrations "$current_version"))

    if [ ${#pending[@]} -eq 0 ]; then
        print_success "没有待执行的迁移"
        return 0
    fi

    # 确认执行
    echo -e "${YELLOW}将执行 ${#pending[@]} 个迁移${NC}"
    read -p "是否继续? [y/N] " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        print_warning "已取消"
        return 0
    fi
    echo ""

    # 备份
    local backup_file=$(backup_database)
    if [ $? -ne 0 ]; then
        print_error "备份失败，终止迁移"
        return 1
    fi
    echo ""

    # 执行迁移
    print_header "执行迁移"
    echo ""

    local success_count=0
    local failed_count=0

    for migration in "${pending[@]}"; do
        local file="$MIGRATIONS_DIR/$migration"

        if execute_migration "$file"; then
            ((success_count++))
        else
            ((failed_count++))
            print_error "迁移失败，停止后续迁移"
            break
        fi

        echo ""
    done

    # 总结
    print_header "迁移完成"
    echo ""
    echo -e "${GREEN}成功: $success_count${NC}"
    if [ $failed_count -gt 0 ]; then
        echo -e "${RED}失败: $failed_count${NC}"
    fi
    echo ""
    echo -e "${BLUE}备份文件: $backup_file${NC}"
    echo -e "${YELLOW}如需回滚，执行:${NC}"
    echo "mysql -h $DB_HOST -u $DB_USER -p'$DB_PASS' --skip-ssl $DB_NAME < $backup_file"
    echo ""

    # 显示最新状态
    show_status
}

# ========================================
# 命令行参数处理
# ========================================

case "${1:-migrate}" in
    "migrate"|"up")
        run_migrations
        ;;
    "status")
        init_migrations_table
        show_status
        ;;
    "history")
        init_migrations_table
        show_history
        ;;
    "help"|"-h"|"--help")
        echo "EzPay 数据库迁移工具"
        echo ""
        echo "用法: $0 [命令]"
        echo ""
        echo "命令:"
        echo "  migrate, up    执行待执行的迁移 (默认)"
        echo "  status         显示当前迁移状态"
        echo "  history        显示迁移历史"
        echo "  help           显示帮助信息"
        echo ""
        ;;
    *)
        print_error "未知命令: $1"
        echo "使用 '$0 help' 查看帮助"
        exit 1
        ;;
esac
