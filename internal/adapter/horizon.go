// Package adapter menerjemahkan respons Horizon ke tipe domain.
//
// Dua jebakan Go yang ditangani di sini, terbukti dari data nyata:
//
//  1. /trades  → price.{n,d} bertipe STRING, field bernama "price"
//     /offers  → price_r.{n,d} bertipe NUMBER, field bernama "price_r"
//     Struct tunggal untuk keduanya gagal unmarshal atau diam-diam nol.
//     flexInt64 menerima kedua bentuk.
//
//  2. Pada /trades, arti price bergantung aset mana yang jadi base. Untuk
//     trade eksploit USTRY/USDC, base adalah USDC dan counter USTRY, jadi
//     price.n/price.d = 0.009369 adalah USTRY-per-USDC — KEBALIKAN dari
//     yang diharapkan. NormalizePrice membalik arah bila perlu.
package adapter

import (
	"fmt"
	"strconv"
	"strings"
)

// flexInt64 unmarshal dari JSON number ATAU JSON string.
type flexInt64 int64

func (f *flexInt64) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fmt.Errorf("flexInt64 %q: %w", s, err)
	}
	*f = flexInt64(v)
	return nil
}

// priceRatio adalah rasio n/d dari Horizon, apa pun tipe JSON aslinya.
type priceRatio struct {
	N flexInt64 `json:"n"`
	D flexInt64 `json:"d"`
}

func (p priceRatio) float() float64 {
	if p.D == 0 {
		return 0
	}
	return float64(p.N) / float64(p.D)
}

// NormalizePrice mengembalikan harga dalam arah counter-per-base untuk
// pasangan yang DIMINTA (quote per base). Jika base pada record adalah aset
// kuotasi (bukan aset dasar yang diminta), harga dibalik.
//
//	raw          : nilai n/d dari record
//	recordBaseIsQuote : true bila base pada record justru aset kuotasi yang diminta
func NormalizePrice(raw float64, recordBaseIsQuote bool) float64 {
	if recordBaseIsQuote {
		if raw == 0 {
			return 0
		}
		return 1.0 / raw
	}
	return raw
}
