// Blend v2 pool census: every pool the v2 pool factory deployed, its on-chain
// status, and what each reserve holds. Read-only (simulated calls only).
//
// Run from keel-mcp (it carries @stellar/stellar-sdk):
//   cd keel-mcp && node ../keel-backend/docs/evidences/2026-11-04-blend-v2-pool-census/census.mjs
import { createRequire } from 'module';
import { writeFileSync } from 'fs';
const require = createRequire(process.cwd() + '/package.json');
const { xdr, scValToNative, nativeToScVal, rpc, Contract, TransactionBuilder, Account, Networks, BASE_FEE } =
  require('@stellar/stellar-sdk');

// blend-capital/blend-utils mainnet.contracts.json, key poolFactoryV2
const FACTORY = 'CDSYOAVXFY7SM5S64IZPPPYB4GVGGLMQVFREPSQQEZVIWXX5R23G4QSU';
const RPC = 'https://mainnet.sorobanrpc.com';
// blend-contracts-v2 pool/src/pool/status.rs
const STATUS = { 0: 'admin active', 1: 'active', 2: 'admin on-ice', 3: 'on-ice', 4: 'admin frozen', 5: 'frozen', 6: 'setup' };
const OUT = new URL('.', import.meta.url).pathname;

const server = new rpc.Server(RPC);
const src = new Account('GAAZI4TCR3TY5OJHCTJC2A4QSY6CJWJH5IAJTGKIN2ER7LBNVKOCCWN7', '0');
async function call(id, fn, ...args) {
  const tx = new TransactionBuilder(src, { fee: BASE_FEE, networkPassphrase: Networks.PUBLIC })
    .addOperation(new Contract(id).call(fn, ...args)).setTimeout(30).build();
  const s = await server.simulateTransaction(tx);
  if (s.error) throw new Error(`${id}.${fn}: ${String(s.error).split('\n')[0]}`);
  return { value: scValToNative(s.result.retval), ledger: s.latestLedger };
}
const scaled = (amount, decimals) => {
  const neg = amount < 0n; let s = (neg ? -amount : amount).toString().padStart(decimals + 1, '0');
  return (neg ? '-' : '') + s.slice(0, -decimals) + (decimals ? '.' + s.slice(-decimals) : '');
};

// The factory has no list function; each pool it deploys leaves one `deploy`
// event. Soroban RPC keeps events ~7 days, so the history comes from the
// stellar.expert index. Hubble `history_contract_events` is the independent check.
const ev = await fetch(`https://api.stellar.expert/explorer/public/contract/${FACTORY}/events?order=asc&limit=200`).then(r => r.json());
const deploys = ev._embedded.records.filter(e => e.topics[0] === 'deploy');

const pools = [], reserves = [];
const symbols = new Map();
for (const e of deploys) {
  const pool = scValToNative(xdr.ScVal.fromXDR(e.bodyXdr, 'base64'));
  const { value: cfg, ledger } = await call(pool, 'get_config');
  const { value: list } = await call(pool, 'get_reserve_list');
  let withSupply = 0;
  for (const asset of list) {
    if (!symbols.has(asset)) symbols.set(asset, (await call(asset, 'symbol').catch(() => ({ value: '?' }))).value);
    const { value: r } = await call(pool, 'get_reserve', nativeToScVal(asset, { type: 'address' }));
    const dec = Number(r.config.decimals);
    const supplied = (BigInt(r.data.b_supply) * BigInt(r.data.b_rate)) / 10n ** 12n;
    const borrowed = (BigInt(r.data.d_supply) * BigInt(r.data.d_rate)) / 10n ** 12n;
    if (supplied > 0n) withSupply++;
    reserves.push({ pool, asset, symbol: symbols.get(asset), supplied: scaled(supplied, dec), borrowed: scaled(borrowed, dec), ledger });
  }
  pools.push({ pool, deployed_utc: new Date(e.ts * 1000).toISOString(), deploy_event: e.id,
    status: Number(cfg.status), status_label: STATUS[cfg.status] ?? 'unknown', reserves: list.length,
    reserves_with_supply: withSupply, read_at_ledger: ledger });
}

const readAt = new Date().toISOString();
writeFileSync(OUT + 'pools.json', JSON.stringify({ factory: FACTORY, rpc: RPC, read_at_utc: readAt, deploy_events: deploys.length, pools, reserves }, null, 2) + '\n');
const csv = (rows) => [Object.keys(rows[0]).join(','), ...rows.map(r => Object.values(r).join(','))].join('\n') + '\n';
writeFileSync(OUT + 'pools.csv', csv(pools));
writeFileSync(OUT + 'reserves.csv', csv(reserves));
console.table(pools.map(({ pool, deployed_utc, status, status_label, reserves, reserves_with_supply, read_at_ledger }) =>
  ({ pool: pool.slice(0, 8), deployed: deployed_utc.slice(0, 10), status, status_label, reserves, reserves_with_supply, read_at_ledger })));
console.table(reserves.filter(r => r.supplied !== '0' && !/^0\.0*$/.test(r.supplied)).map(r => ({ pool: r.pool.slice(0, 8), symbol: r.symbol, supplied: r.supplied, borrowed: r.borrowed })));
