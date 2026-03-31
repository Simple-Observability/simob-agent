# Development guide

## Adding a new metrics collector
### Naming conventions
Metric names follow a mandatory prefix and suffix convention:
- **Prefix**: The unique source name (e.g., `postgresql_`, `nginx_`).
- **Suffix**: Indicates the nature of the metric:
  - `_total`: Monotonic counters (raw values).
  - `_rate`: Values computed as deltas over time (per second).
  - `_ratio`: Values representing a percentage or fraction.
  - `_bytes`: Byte counts (size).
  - `_bps`: Throughput (bytes or bits per second).
  - `_ms`: Time durations.

### Units
The unit must be one of the following supported values:
- `no`: No unit (default for counts).
- `%`: Percent.
- `bytes`: Bytes.
- `bps`: Bytes per second.
- `rate`: Rate (per second).
- `ms`: Milliseconds.

## Conventions and good practices
- **Compute rates in the agent**: Prefer computing rates within the agent rather than sending raw counters. This ensures metrics are immediately useful for monitoring.
- **Use BaseCollector**: Embed `metrics.BaseCollector` in your collector struct.
- **Interface-based sources**: Use interfaces for underlying data sources (API, DB, etc.) to enable easy unit testing with mocks.

### Checklist
- [ ] Implement collector in `agent/internal/metrics/[plugin]/`
- [ ] Register the new collector in `agent/internal/metrics/registry.go`
