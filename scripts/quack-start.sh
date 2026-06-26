#!/bin/bash
# quack-start.sh — One-click DuckDB Quack server starter
set -e

DATA_DIR="${1:-./quack-data}"
TOKEN="${2:-quack_$(date +%s)}"
PORT="${3:-9491}"
DB="${DATA_DIR}/default.db"

mkdir -p "$DATA_DIR"

echo "🚀 Starting Quack server"
echo "   Database: $DB"
echo "   Port:     $PORT"
echo "   Token:    $TOKEN"
echo ""

# Clean up old processes
pkill -f "duckdb.*quack\|duckdb.*$PORT" 2>/dev/null || true
sleep 1

# Start DuckDB + Quack (keep process alive)
(echo "CALL quack_serve('quack:localhost:$PORT', token := '$TOKEN');" && cat) | \
    duckdb "$DB" > /dev/null 2>&1 &

sleep 2

# Verify
if ! ss -tlnp 2>/dev/null | grep -q ":$PORT"; then
    echo "❌ Startup failed, port $PORT not listening"
    exit 1
fi

echo "✅ Quack server is running"
echo ""
echo "📋 Connection commands:"
echo ""
echo "  duckdb -c \""
echo "    CREATE SECRET (TYPE QUACK, TOKEN '$TOKEN');"
echo "    ATTACH 'quack:localhost:$PORT' AS remote;"
echo "  \""
echo ""
echo "🛑 Stop: pkill -f 'duckdb.*$DB'"
echo "📁 Token saved to: $DATA_DIR/.token"
echo "$TOKEN" > "$DATA_DIR/.token"
echo "$(ps aux | grep "duckdb.*$DB" | grep -v grep | awk '{print $2}')" > "$DATA_DIR/.pid"