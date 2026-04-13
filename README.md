# CryptoSim

A crypto exchange simulator in Go. Runs a live order matching engine anchored to real Coinbase prices, with competing autonomous market maker and trader agents communicating over NATS. Also persists trades and orderbook snapshots to PostreSQL database (with backpressure handling)

[Design Document](https://docs.google.com/document/d/1wUNIaR00qRUM6UEflLkuUxJdHGq335RwkgV1zlNsWiQ/edit?usp=sharing).

---

## Stack

- Go
- NATS (pub/sub + request-reply)
- Coinbase Advanced Trade API (WebSocket)
- PostgreSQL (TimescaleDB)
- Docker

---

<img width="2520" height="2171" alt="CryptoSim(1)" src="https://github.com/user-attachments/assets/076d7fa4-4f87-4740-bc73-820df02c53e5" />
![cryptosim](assets/demo.gif)

<img width="1070" height="930" alt="Screenshot 2026-04-05 at 06 55 20" src="https://github.com/user-attachments/assets/9345a738-d682-471c-877e-7ad0df8e11f3" />
<img width="1066" height="925" alt="Screenshot 2026-04-05 at 06 55 45" src="https://github.com/user-attachments/assets/83b3311e-22c2-4a29-adb9-463b8d4257c7" />
<img width="1067" height="926" alt="Screenshot 2026-04-05 at 06 56 10" src="https://github.com/user-attachments/assets/a92bf841-2283-40c2-bf6f-fb745ddf32a7" />
<img width="1055" height="931" alt="Screenshot 2026-04-05 at 06 56 26" src="https://github.com/user-attachments/assets/48fd2635-3dcd-4089-abfd-50b95679391d" />



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

## How It Works
The matching engine maintains three independent order books (BTC-USD, ETH-USD, XRP-USD), each backed by a max-heap (bids) and min-heap (asks) for price-time priority matching. Supports limit and market orders with partial fills. Agents submit orders via NATS request-reply for synchronous acknowledgment; price feeds and trade events fan out over pub/sub.
Market Makers provide liquidity using three different strategies: a fixed-spread scalper (BTC), a momentum-skewed quoter (ETH), and an Avellaneda-Stoikov optimal market maker with dynamic inventory management (XRP).
Traders consume liquidity via momentum following (ETH), mean reversion ladders (BTC), VWAP-based execution using live trade data (BTC), and random noise orders across all three coins to prevent book stalling.
Persistence writes 57K+ rows/sec using PostgreSQL's binary COPY protocol (pgx.CopyFrom) with 8 worker goroutines. A circular buffer catches batches during transient DB failures to keep the hot path non-blocking.
For the full breakdown — matching logic, strategy math, NATS channel design, data model, and tradeoff analysis — see the [Design Document](https://docs.google.com/document/d/1wUNIaR00qRUM6UEflLkuUxJdHGq335RwkgV1zlNsWiQ/edit?usp=sharing).


## Getting Started

**Prerequisites:** Docker + Coinbase Advanced Trade API key

```bash
git clone https://github.com/dikpaal/CryptoSim.git
cd CryptoSim
cp .env.example .env  # add Coinbase credentials
make build && make up
```

In a separate terminal:
```bash
go run ./cmd/tui
```

---

## Project Structure

```
CryptoSim/
├── cmd/                # Entry points (engine, agents, TUI, persistence)
├── internal/           # Core packages (orderbook, matching, agents, pricing)
├── persistence/        # DB write path, circular buffer, batch workers
├── migrations/         # Embedded SQL for TimescaleDB hypertable setup
├── assets/             # Demo gif, screenshots
├── docker-compose.yml
├── Dockerfile
├── Makefile
└── LOAD_TESTING.md     # Load testing methodology and results
```
