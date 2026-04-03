# Load Testing Guide

## Quick Start

```bash
# Build and start all services
docker compose up -d --build

# Run load tester
docker compose --profile testing up load-tester

# Stop load tester
docker compose --profile testing down

# Stop everything
docker compose down
```

## Configuration

Edit `docker-compose.yml` under `load-tester` service:

```yaml
environment:
  - NATS_URL=nats://nats:4222          # NATS connection
  - NUM_WORKERS=50                      # Concurrent workers
  - ORDERS_PER_SECOND=20000             # Target throughput
```

### Scaling Options

**Light load:**
```yaml
NUM_WORKERS=10
ORDERS_PER_SECOND=5000
```

**Medium load:**
```yaml
NUM_WORKERS=50
ORDERS_PER_SECOND=20000
```

**Heavy load:**
```yaml
NUM_WORKERS=100
ORDERS_PER_SECOND=50000
```

## Metrics

Reports every 5 seconds:
- **Orders submitted/s** - actual throughput (target: 15k+)
- **Orders accepted/s** - successfully processed
- **Orders rejected** - failures
- **Trades/s** - executed trades (target: 25k+)
- **Avg latency** - order round-trip time

## Sample Output

```
Orders: 491377 submitted (19711/s), 491377 accepted (19711/s), 0 rejected | Trades: 387045 (15533/s) | Avg Latency: 1.08ms
```

## Scale Participants

```bash
# More market makers = more liquidity
docker compose up -d --scale scalper-mm=3 --scale momentum-mm=2

# More traders = more activity
docker compose up -d --scale vwap-trader=5 --scale noise-trader=3
```

## Monitoring

```bash
# Watch all logs
docker compose logs -f

# Watch specific service
docker compose logs -f engine
docker compose logs -f load-tester

# NATS monitoring
open http://localhost:8222
```
