# YELLOW ZONE: internal/hubble

Adapter for historical data from Hubble (the BigQuery dataset
`crypto-stellar.crypto_stellar`).

The output MUST be a `domain.Snapshot`, identical to internal/horizon.

## Cost constraints

This runs on the BigQuery Sandbox: 1TB of query per month, free, no billing.
Every query MUST:

- filter on the partition (bound the ledger range or the date) before anything else
- select columns explicitly, never SELECT *
- be preceded by a dry run to check how many bytes it will scan

A query with no partition filter can scan hundreds of gigabytes in one go.

## Rules

Never write an asset issuer address from memory. If you need an asset identity,
take it from `docs/decisions/` or ask Al to confirm it from a primary source
first.

## Status

Deferred. See `docs/decisions/DEC-002-hold-bigquery.md`: only one thing is blocked
without BigQuery, the orderbook state at a past ledger, and there are three
cheaper substitutes ahead of it in the queue.
