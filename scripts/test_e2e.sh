#!/bin/bash
set -e

ENGINE_URL="http://localhost:8081"
DB_HOST="localhost"
DB_PORT="6543"
DB_NAME="cryptosim"
DB_USER="postgres"
DB_PASS="password"

echo "=== Phase 2: End-to-End Test ==="
echo

# Test 1: Submit matching orders
echo "Test 1: Submitting matching BUY and SELL orders..."

BUY_ORDER=$(curl -s -X POST "$ENGINE_URL/orders" \
  -H "Content-Type: application/json" \
  -d '{
    "mm_id": "test-mm-buyer",
    "symbol": "BTC-USD",
    "side": "BID",
    "order_type": "LIMIT",
    "price": 50000.00,
    "qty": 0.1
  }')

echo "BUY order response: $BUY_ORDER"
BUY_ORDER_ID=$(echo $BUY_ORDER | grep -o '"order_id":"[^"]*"' | cut -d'"' -f4)

SELL_ORDER=$(curl -s -X POST "$ENGINE_URL/orders" \
  -H "Content-Type: application/json" \
  -d '{
    "mm_id": "test-mm-seller",
    "symbol": "BTC-USD",
    "side": "ASK",
    "order_type": "LIMIT",
    "price": 50000.00,
    "qty": 0.1
  }')

echo "SELL order response: $SELL_ORDER"
SELL_ORDER_ID=$(echo $SELL_ORDER | grep -o '"order_id":"[^"]*"' | cut -d'"' -f4)

echo
echo "Waiting 3 seconds for persistence..."
sleep 3

# Test 2: Verify trade in TimescaleDB
echo
echo "Test 2: Checking TimescaleDB for trade..."

TRADE_COUNT=$(PGPASSWORD=$DB_PASS psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -t -c \
  "SELECT COUNT(*) FROM trades WHERE buyer_order_id = '$BUY_ORDER_ID' OR seller_order_id = '$SELL_ORDER_ID';")

TRADE_COUNT=$(echo $TRADE_COUNT | xargs)

if [ "$TRADE_COUNT" -gt 0 ]; then
  echo "✓ Found $TRADE_COUNT trade(s) in TimescaleDB"

  echo
  echo "Trade details:"
  PGPASSWORD=$DB_PASS psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -c \
    "SELECT trade_id, symbol, price, qty, buyer_mm_id, seller_mm_id, executed_at
     FROM trades
     WHERE buyer_order_id = '$BUY_ORDER_ID' OR seller_order_id = '$SELL_ORDER_ID'
     ORDER BY executed_at DESC
     LIMIT 5;"

  echo
  echo "✓ End-to-end test PASSED"
  exit 0
else
  echo "✗ No trades found in TimescaleDB"
  echo "✗ End-to-end test FAILED"
  exit 1
fi
