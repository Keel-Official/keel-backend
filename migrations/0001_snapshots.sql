-- Snapshot orderbook + AMM. Satu baris per pengambilan per pasangan aset.

CREATE TABLE IF NOT EXISTS snapshots (
    id                  BIGSERIAL PRIMARY KEY,
    captured_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    ledger_seq          BIGINT      NOT NULL,
    methodology_version TEXT        NOT NULL,
    source              TEXT        NOT NULL CHECK (source IN ('horizon', 'hubble')),
    base_asset          TEXT        NOT NULL,
    counter_asset       TEXT        NOT NULL,
    raw                 JSONB       NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_snapshots_pair_time
    ON snapshots (base_asset, counter_asset, captured_at DESC);

CREATE INDEX IF NOT EXISTS idx_snapshots_ledger
    ON snapshots (ledger_seq);

-- Level orderbook yang sudah dinormalisasi.
-- Harga disimpan sebagai pecahan (price_n / price_d) supaya tidak ada
-- kehilangan presisi. JANGAN menambahkan kolom float di sini.

CREATE TABLE IF NOT EXISTS snapshot_levels (
    id           BIGSERIAL PRIMARY KEY,
    snapshot_id  BIGINT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
    venue        TEXT   NOT NULL CHECK (venue IN ('sdex', 'amm')),
    side         TEXT   NOT NULL CHECK (side IN ('bid', 'ask')),
    price_n      BIGINT NOT NULL,
    price_d      BIGINT NOT NULL CHECK (price_d > 0),
    amount       NUMERIC(38, 18) NOT NULL,
    level_index  INT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_levels_snapshot
    ON snapshot_levels (snapshot_id, venue, side, level_index);
