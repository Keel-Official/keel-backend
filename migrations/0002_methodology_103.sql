-- Columns that methodology 1.0.3 added to the output type.
--
-- WHY THIS FILE EXISTS. The "not here" note at the bottom of 0001_core.sql said
-- the two C_max terms needed a change to the type first and a migration here
-- second, and that adding columns for values nothing produces would be clutter
-- reading like a promise. The type changed in 1.0.3, so this is the second half
-- of that sentence being kept.
--
-- WHAT IS DELIBERATELY NOT HERE. The manipulation cost split, combined against
-- orderbookOnly, is not in this file. Whether manipulation_cost stays as the
-- combined ladder and gains a sibling column, or is replaced by two columns and
-- dropped, is the same question the API contract has to answer. Migrating it
-- twice in two directions is worse than migrating it once in the right one. See
-- handoff item 14.
--
-- ADD COLUMN IF NOT EXISTS, matching the idiom in 0001. Together with the
-- bookkeeping in scripts/migrate.sh that makes a second application harmless.
-- Without the bookkeeping it would make a second application SILENT, which is
-- the failure this repository keeps finding, so the bookkeeping is the thing
-- carrying the guarantee here, not the IF NOT EXISTS.

-- The pool's marginal price, Y over X, from the pool with the largest quote
-- reserve. Populated whenever an active pool exists, regardless of which branch
-- the P0 rule took, because a consumer comparing the two sources needs both
-- numbers and not only the one that won.
ALTER TABLE metrics ADD COLUMN IF NOT EXISTS pool_spot_price NUMERIC;

-- In PERCENT, matching every quantity whose name ends in Pct. This is the
-- distance between the order book mid and the pool spot. Above
-- Thresholds.PriceDivergencePct the flag PRICE_SOURCE_CONFLICT fires and P0 is
-- taken from the pool. NULL when there is no active pool, which means undefined
-- rather than zero: an asset with no pool has no divergence to report, and zero
-- would claim the two sources agree.
ALTER TABLE metrics ADD COLUMN IF NOT EXISTS price_divergence_pct NUMERIC;

-- The two terms behind max_safe_collateral, which methodology section 9 requires
-- to be reported separately rather than only as their minimum. Reporting only
-- the minimum hides WHICH constraint binds, and that is the part a lender acts
-- on: a position limited by liquidation depth and one limited by manipulation
-- cost call for different responses.
--
--   max_safe_collateral_liquidation  = D_sell(delta_liquidation) x h
--   max_safe_collateral_manipulation = MC_orderbookOnly(delta_critical) x m
--
-- The manipulation term is NULL when the critical target is unreachable through
-- the order book. Per section 9 the term is then not applied at all, C_max falls
-- back to the liquidation term alone, and a warning is emitted. NULL here means
-- "not applicable", which is the same nil convention the output type uses, and
-- it is NOT the same as zero: zero would claim the attack is free.
ALTER TABLE metrics ADD COLUMN IF NOT EXISTS max_safe_collateral_liquidation  NUMERIC;
ALTER TABLE metrics ADD COLUMN IF NOT EXISTS max_safe_collateral_manipulation NUMERIC;
