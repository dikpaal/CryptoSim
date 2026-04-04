# CryptoSim

A crypto exchange simulator in Go. Runs a live order matching engine anchored to real Coinbase prices, with competing autonomous market maker and trader agents communicating over NATS.
<img width="2520" height="2171" alt="CryptoSim(1)" src="https://github.com/user-attachments/assets/076d7fa4-4f87-4740-bc73-820df02c53e5" />
![cryptosim](https://github.com/user-attachments/assets/d003a796-ed88-4475-8f89-2cf7809dcda3)

---

## Load Testing Results

| Metric | Result |
|---|---|
| Order throughput | 32,000+ orders/s |
| Trade throughput | 40,000+ trades/s |
| Average order latency | 1.2ms |
| Rejections | 0 |
| DB write throughput | 57,000+ writes/s |

---

## Stack

| Layer | Used |
|---|---|
| Engine & participants | Go |
| Service Communication | NATS (pub/sub + request-reply) |
| Price feed | Coinbase Advanced Trade API (WebSocket) |
| Persistence | TimescaleDB (PostgreSQL) |
| Infrastructure | Docker |

---

## How the Engine Works

The matching engine maintains three independent order books (BTC-USD, ETH-USD, XRP-USD), each backed by a `MaxHeap` (bids) and `MinHeap` (asks) for O(log n) best-price access.

**Order flow:**

1. A participant submits an order via NATS **request-reply** on `orders.submit`.
2. The engine unmarshals the order, routes it to the correct order book, and runs the matching algorithm.
3. **Limit orders** walk the opposite side: a bid matches against the best ask while `bid.price ≥ ask.price`; fills happen at the resting order's price (price-time priority).
4. **Market orders** consume the opposite side until filled or the book is exhausted; an unfilled market order is cancelled (never rests).
5. Partial fills are supported — unfilled limit remainder rests on the book.
6. After matching, the engine publishes each resulting trade to `trades.executed` and a top-10 order book snapshot to `orderbook.snapshot`, then ACKs the submitter synchronously.

Cancel requests follow the same request-reply pattern on `orders.cancel`.

---

## Participants

7 participants run as independent Go services, each subscribing to a Coinbase price feed and submitting orders to the engine.

### Market Makers

#### MM1 — Scalper (BTC-USD)

Quotes a tight fixed spread around mid, across N price levels. Requotes only when the mid moves more than `minMoveThresh`.

$$b^i = m - \left(\frac{s}{2} + i \cdot \Delta\right), \quad a^i = m + \left(\frac{s}{2} + i \cdot \Delta\right)$$

where $s = m \cdot \text{spreadBps} \times 10^{-4}$ and $\Delta = m \cdot \text{levelSpacing} \times 10^{-4}$

Parameters: `spreadBps=2`, `numLevels=3`, `orderSize=0.01 BTC`.

---

#### MM2 — Momentum Market Maker (ETH-USD)

Like the scalper but skews the entire quote ladder in the direction of recent price movement. When price is rising, quotes shift up — the MM buys cheaper and sells into strength.

$$\phi = (m_t - m_{t-1}) \cdot \alpha$$

$$b^i = m - \text{offset}_i + \phi, \quad a^i = m + \text{offset}_i + \phi$$

where $\alpha = 0.3$ is the skew factor

Parameters: `spreadBps=4`, `skewFactor=0.3`, `numLevels=5`, `orderSize=0.1 ETH`.

---

#### MM3 — Avellaneda-Stoikov Market Maker (XRP-USD)

Implements the Avellaneda-Stoikov (2008) optimal market making model. The reservation price adjusts for inventory risk; the spread widens with volatility and inventory.

$$r = m - q \gamma \sigma^2 T$$

$$\delta^{\ast} = \gamma \sigma^2 T + \frac{2}{\gamma} \ln\!\left(1 + \frac{\gamma}{\kappa}\right)$$

$$b^i = r - \frac{\delta^{\ast}}{2} - i\Delta, \quad a^i = r + \frac{\delta^{\ast}}{2} + i\Delta$$

where $q$ = inventory, $\gamma$ = risk aversion, $\sigma$ = volatility, $T$ = time horizon, $\kappa$ = order arrival intensity.

$\kappa$ is updated dynamically from the live order book:

$$\kappa = \ln\!\left(1 + \bar{V}_{\text{best}}\right)$$

Parameters: `γ=0.1`, `κ=1.5` (dynamic), `σ=0.02`, `T=1.0`, `numLevels=5`, `orderSize=50 XRP`.

---

### Traders

#### T1 — Momentum Trader (ETH-USD)

Maintains a rolling price window of size N. When the window shows a trend exceeding a threshold, it enters a directional position and places a take-profit limit.

$$\tau = p_t - p_{t-N}$$

$$\text{if } \tau > \theta: \text{ buy at } m, \text{ take-profit sell at } m(1 + 0.002)$$
$$\text{if } \tau < -\theta: \text{ sell at } m, \text{ take-profit buy at } m(1 - 0.002)$$

Parameters: `windowSize=10`, `threshold=0.2%`, `orderSize=0.05 ETH`.

---

#### T2 — Mean Reversion Trader (BTC-USD)

Places a symmetric limit order ladder around the current mid. Does nothing until price drifts far enough from the base to warrant rebuilding.

$$b^i = m - (i+1)\Delta, \quad a^i = m + (i+1)\Delta, \quad i = 0, \ldots, N-1$$

Ladder rebuilds when $\dfrac{|m - m_0|}{m_0} > \theta_{\text{rebuild}}$

Parameters: `levels=5`, `spacing=0.1%×mid`, `rebuildThresh=0.5%`, `orderSize=0.05 BTC`.

---

#### T3 — Noise Trader (XRP-USD)

Places random orders on a random interval. Provides background liquidity consumption.

$$\text{side} \sim \text{Bernoulli}(0.5), \quad \text{type} \sim \text{Bernoulli}(0.5)$$

$$q \sim \mathcal{U}(0.001,\ 0.05), \quad p = m \pm \mathcal{U}(0,\ 0.001) \cdot m, \quad \Delta t \sim \mathcal{U}(50\text{ms},\ 500\text{ms})$$

---

#### T4 — VWAP Trader (BTC-USD)

Computes a rolling volume-weighted average price over the last 50 trades. Buys when price is below VWAP, sells when above.

$$\text{VWAP} = \frac{\sum_{i=1}^{N} p_i q_i}{\sum_{i=1}^{N} q_i}, \quad N = 50$$

$$d = \frac{m - \text{VWAP}}{\text{VWAP}}$$

$$d < -\theta \Rightarrow \text{buy limit at } m \qquad d > +\theta \Rightarrow \text{sell limit at } m$$

Parameters: `window=50`, `threshold=0.1%`, `orderSize=0.05 BTC`.

---

## Persistence

Three TimescaleDB hypertables are created at startup via embedded SQL migrations:

**`trades`** — partitioned by `executed_at`
```sql
trade_id        UUID
symbol          TEXT
price           NUMERIC(20, 8)
qty             NUMERIC(20, 8)
buyer_mm_id     TEXT
seller_mm_id    TEXT
buyer_order_id  UUID
seller_order_id UUID
executed_at     TIMESTAMPTZ      -- partition key
```

**`orderbook_snapshots`** — partitioned by `snapshot_at`
```sql
id          BIGSERIAL
symbol      TEXT
bids        JSONB            -- top-N levels [price, qty]
asks        JSONB
mid_price   NUMERIC(20, 8)
spread      NUMERIC(20, 8)
snapshot_at TIMESTAMPTZ      -- partition key
```
Index: `(symbol, snapshot_at DESC)`

**`mm_status`** — partitioned by `recorded_at`
```sql
id              BIGSERIAL
mm_id           TEXT
strategy        TEXT
inventory       NUMERIC(20, 8)
realized_pnl    NUMERIC(20, 8)
unrealized_pnl  NUMERIC(20, 8)
open_orders     INT
recorded_at     TIMESTAMPTZ      -- partition key
```
Index: `(mm_id, recorded_at DESC)`

**Write path:**
The persistence service subscribes to `trades.executed`. Incoming trades are fanned to 8 worker goroutines via a buffered channel (capacity 150k). Each worker accumulates trades and flushes using `pgx.CopyFrom` (PostgreSQL binary copy protocol) either every 100ms or when the batch hits 5,000 rows — whichever comes first. This is what achieves 57k+ writes/s.

**Circular buffer for backpressure:**
If a DB write fails (e.g., transient connection loss), the batch is written to an in-memory circular buffer (capacity 100k trades) instead of being dropped. On the next successful flush, the circular buffer is drained first. The buffer overwrites oldest entries if it fills — a deliberate trade-off that keeps the hot path non-blocking at the cost of data loss only under sustained DB outage.
