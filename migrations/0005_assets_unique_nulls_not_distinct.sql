-- The assets uniqueness constraint did not constrain the native asset, and the
-- code that depends on it says in its own comment that it does.
--
-- WHAT WAS WRONG. 0001 declared:
--
--     UNIQUE (code, issuer, quote_code, quote_issuer)
--
-- In SQL, NULL is not equal to NULL, so a unique index treats two rows carrying a
-- NULL in an indexed column as distinct. The native asset is the one asset with no
-- issuer, and assets_native_has_no_issuer REQUIRES that issuer be NULL for it, so
-- every XLM row is unique against every other XLM row by construction. The
-- constraint was never able to bind on the only asset it was named after.
--
-- WHAT THAT BROKE, in two places that each believed the other was safe:
--
--   internal/store/assets.go   its comment reads "the constraint is what stops a
--                              native asset from being stored twice under two
--                              spellings". It stopped nothing.
--   UpsertAsset                ON CONFLICT (code, issuer, quote_code, quote_issuer)
--                              never fires for a native base, so the statement
--                              falls through to a plain INSERT. `keel assets
--                              -pairs` is documented as idempotent and was, for
--                              every asset except XLM, which gained one new row per
--                              run.
--
-- FOUND ON 26 AUGUST 2026 by scanning a 60 pair demonstration set and reading the
-- results ordered by maxSafeCollateral: XLM appeared twice, at two different asset
-- ids, with two slightly different spreads because the two rows were written a few
-- ledgers apart. A duplicate that produced IDENTICAL numbers would have been
-- invisible. Nothing in the test suite covered it either, because internal/store's
-- integration tests declare credit assets.
--
-- THE FIX IS NULLS NOT DISTINCT, available since Postgres 15 and this project runs
-- 16. It makes the index treat two NULLs as equal, which is what every caller
-- already assumed. The rejected alternative was a unique index over
-- COALESCE(issuer, ''): it works on every version, and it hides the intent behind
-- an expression that a reader has to decode, while NULLS NOT DISTINCT says the
-- thing out loud. The other rejected alternative, storing the native issuer as an
-- empty string, contradicts assets_native_has_no_issuer and would push the same
-- ambiguity down into every query instead of removing it.

BEGIN;

-- 1. Merge the duplicates that the missing constraint already allowed in.
--
-- The SURVIVOR is the lowest id for each identity, because metrics rows already
-- point at it and the oldest row carries the longest history. Identity here spells
-- NULL out as an empty string deliberately: this statement has to group the very
-- rows the broken index refused to group.
CREATE TEMP TABLE asset_merge ON COMMIT DROP AS
SELECT a.id AS dup_id,
       MIN(a.id) OVER (
         PARTITION BY a.code, COALESCE(a.issuer, ''), a.quote_code, COALESCE(a.quote_issuer, '')
       ) AS keep_id
FROM assets a;

DELETE FROM asset_merge WHERE dup_id = keep_id;

-- 2. Move the duplicates' metrics onto the survivor.
--
-- A metrics row is unique on (asset_id, ledger_seq, methodology_version,
-- data_source), so a duplicate asset scanned in the SAME round as its survivor
-- collides on the way over. That collision is not a loss: both rows describe one
-- ledger of one asset computed by one methodology from one source, so they are the
-- same measurement recorded twice. The colliding rows are dropped in step 3 rather
-- than merged, and any row that does not collide is carried across.
UPDATE metrics m
   SET asset_id = am.keep_id
  FROM asset_merge am
 WHERE m.asset_id = am.dup_id
   AND NOT EXISTS (
         SELECT 1 FROM metrics k
          WHERE k.asset_id            = am.keep_id
            AND k.ledger_seq          = m.ledger_seq
            AND k.methodology_version = m.methodology_version
            AND k.data_source         = m.data_source
       );

-- 3. Drop what could not be carried across, then the duplicate assets themselves.
DELETE FROM metrics m USING asset_merge am WHERE m.asset_id = am.dup_id;
DELETE FROM assets  a USING asset_merge am WHERE a.id       = am.dup_id;

-- 4. Replace the constraint with one that binds on the native asset too.
ALTER TABLE assets DROP CONSTRAINT assets_code_issuer_quote_code_quote_issuer_key;

ALTER TABLE assets
  ADD CONSTRAINT assets_code_issuer_quote_code_quote_issuer_key
  UNIQUE NULLS NOT DISTINCT (code, issuer, quote_code, quote_issuer);

COMMIT;
