#!/bin/bash
set -e

ENGINE_URL="http://localhost:8081"

echo "=== Phase 2: Orderbook Snapshot Test ==="
echo

# Test 1: Submit non-matching orders
echo "Test 1: Submitting non-matching orders..."

curl -s -X POST "$ENGINE_URL/orders" \
  -H "Content-Type: application/json" \
  -d '{
    "mm_id": "test-mm-bid-1",
    "symbol": "BTC-USD",
    "side": "BID",
    "order_type": "LIMIT",
    "price": 49000.00,
    "qty": 0.5
  }' | jq

curl -s -X POST "$ENGINE_URL/orders" \
  -H "Content-Type: application/json" \
  -d '{
    "mm_id": "test-mm-bid-2",
    "symbol": "BTC-USD",
    "side": "BID",
    "order_type": "LIMIT",
    "price": 48500.00,
    "qty": 0.3
  }' | jq

curl -s -X POST "$ENGINE_URL/orders" \
  -H "Content-Type: application/json" \
  -d '{
    "mm_id": "test-mm-ask-1",
    "symbol": "BTC-USD",
    "side": "ASK",
    "order_type": "LIMIT",
    "price": 51000.00,
    "qty": 0.4
  }' | jq

curl -s -X POST "$ENGINE_URL/orders" \
  -H "Content-Type: application/json" \
  -d '{
    "mm_id": "test-mm-ask-2",
    "symbol": "BTC-USD",
    "side": "ASK",
    "order_type": "LIMIT",
    "price": 51500.00,
    "qty": 0.2
  }' | jq

echo
echo "Test 2: Fetching orderbook snapshot from matching engine..."

ORDERBOOK=$(curl -s "$ENGINE_URL/orderbook")
echo $ORDERBOOK | jq

BIDS_COUNT=$(echo $ORDERBOOK | jq '.bids | length')
ASKS_COUNT=$(echo $ORDERBOOK | jq '.asks | length')

echo
if [ "$BIDS_COUNT" -gt 0 ] && [ "$ASKS_COUNT" -gt 0 ]; then
  echo "✓ Orderbook has $BIDS_COUNT bids and $ASKS_COUNT asks"
  echo "✓ Orderbook snapshot test PASSED"
  exit 0
else
  echo "✗ Orderbook is empty or incomplete"
  echo "✗ Orderbook snapshot test FAILED"
  exit 1
fi
