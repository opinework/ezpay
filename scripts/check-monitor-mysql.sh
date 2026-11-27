#!/bin/bash
set -euo pipefail
script_dir="$(cd "$(dirname "$0")" && pwd)"
if [[ -z "${TEST_RUN_DIR:-}" ]]; then
    exec /Users/zeno/.local/bin/test-on-external bash "$script_dir/check-monitor-mysql.sh" "$@"
fi
mysql_server=/opt/homebrew/bin/mysqld
mysql_admin=/opt/homebrew/bin/mysqladmin
if [[ ! -x "$mysql_server" || ! -x "$mysql_admin" ]]; then
    echo 'Local MySQL server tools are required for this isolated test.' >&2
    exit 1
fi
mysql_data="$TMPDIR/mysql-data"
mysql_temp="$TMPDIR/mysql-tmp"
mysql_files="$TMPDIR/mysql-files"
mysql_socket="$TEST_RUN_DIR/mysql.sock"
mysql_pid=
cleanup_mysql() {
    if [[ -n "$mysql_pid" ]]; then
        kill -TERM "$mysql_pid" 2>/dev/null || true
        wait "$mysql_pid" 2>/dev/null || true
    fi
}
trap cleanup_mysql EXIT
mkdir -p "$mysql_data" "$mysql_temp" "$mysql_files"
"$mysql_server" --no-defaults --initialize-insecure --datadir="$mysql_data" \
    --log-error="$TEST_ARTIFACTS_DIR/mysql-init.log" > "$TEST_ARTIFACTS_DIR/mysql-init.stdout" 2>&1
"$mysql_server" --no-defaults --datadir="$mysql_data" --skip-networking --mysqlx=OFF \
    --socket="$mysql_socket" --pid-file="$TMPDIR/mysql.pid" --tmpdir="$mysql_temp" \
    --secure-file-priv="$mysql_files" --skip-log-bin --innodb-buffer-pool-size=64M \
    --innodb-redo-log-capacity=32M --log-error="$TEST_ARTIFACTS_DIR/mysql.log" \
    > "$TEST_ARTIFACTS_DIR/mysql.stdout" 2>&1 &
mysql_pid=$!
ready=0
for ((attempt=0; attempt<60; attempt++)); do
    if "$mysql_admin" --no-defaults --protocol=SOCKET --socket="$mysql_socket" -uroot ping >/dev/null 2>&1; then
        ready=1
        break
    fi
    if ! kill -0 "$mysql_pid" 2>/dev/null; then break; fi
    sleep 0.5
done
if [[ "$ready" != 1 ]]; then
    echo "Temporary MySQL failed to start; see $TEST_ARTIFACTS_DIR/mysql.log" >&2
    exit 1
fi
export EZPAY_MONITOR_TEST_MYSQL_SOCKET="$mysql_socket"
export GOCACHE="$XDG_CACHE_HOME/ezpay-go-build"
export GOMODCACHE="$XDG_CACHE_HOME/ezpay-go-mod"
cd "$script_dir/.."
go test -race -count=1 -json ./internal/handler -run '^TestMonitorMySQLTransactions$' \
    | tee "$TEST_ARTIFACTS_DIR/mysql-tests.json"
