-- The two columns 0002 deliberately left out, and one new data source value.
--
-- WHY THESE WAITED. 0002 added the four columns whose names were already settled
-- in the output type and stopped there, because whether manipulation_cost stayed
-- as the combined ladder and gained a sibling, or was replaced by two columns,
-- was the same question the API contract had to answer. Contract 1.3.0 answered
-- it: manipulationCostCombined and manipulationCostOrderbookOnly, with the old
-- name retired rather than kept as an alias. One name for one thing.
--
-- A RENAME AND NOT A NEW COLUMN. manipulation_cost held the combined ladder
-- already, so this is the same data under an accurate name. It is free today
-- because the table is empty; it would not be free later, and that is the reason
-- to do it now rather than to keep the old name out of caution.
--
-- Note that ALTER TABLE ... RENAME COLUMN has no IF NOT EXISTS, unlike every
-- statement in 0001 and 0002. Re-running this file therefore FAILS rather than
-- passing silently, and the bookkeeping in scripts/migrate.sh is what stops it
-- being re-run. That is the right way round: a rename that silently did nothing
-- the second time would hide a broken schema_migrations table, which is exactly
-- the failure this repository keeps finding.

ALTER TABLE metrics RENAME COLUMN manipulation_cost TO manipulation_cost_combined;

-- [{delta, targetPrice, cost, reachable}] counting the ORDERBOOK ONLY.
--
-- orderbook_only <= combined always holds, because combined is the same book plus
-- an AMM term. An attacker takes the cheapest path, so this is the binding figure
-- and the one behind max_safe_collateral_manipulation.
--
-- The GAP between the two columns is itself the signal, and it is the reason this
-- is a separate column rather than a derived view. An asset with a large combined
-- cost and a small orderbook-only cost looks defended and is not. On 22 February
-- 2026 an honest pool held USTRY at 1.0555 throughout, moving the real market
-- price to 106.74 would have cost about 147.96 USDC, and the attacker paid zero by
-- using the orderbook alone.
ALTER TABLE metrics ADD COLUMN IF NOT EXISTS manipulation_cost_orderbook_only JSONB;

-- offers-implied: the book was reconstructed by replaying manage_sell_offer and
-- manage_buy_offer operations rather than read from an endpoint. It ranks ABOVE
-- trades-implied in confidence, because an offer proves liquidity that was POSTED
-- while a trade proves only liquidity that was CONSUMED. The golden fixture is
-- this value, not horizon, which is handoff item 5b.
--
-- Dropping and recreating rather than altering: a CHECK constraint has no ALTER
-- form. Safe on any number of rows here, because the new set is a superset of the
-- old one, so no existing row can fail the new constraint.
ALTER TABLE metrics DROP CONSTRAINT IF EXISTS metrics_data_source_check;
ALTER TABLE metrics ADD CONSTRAINT metrics_data_source_check
    CHECK (data_source IN ('horizon', 'hubble', 'offers-implied', 'trades-implied'));
