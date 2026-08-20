#!/usr/bin/env bash
#
# lindungi-zona-merah.sh
#
# Hook PreToolUse untuk Bash. Menolak perintah yang akan MENGUBAH berkas di
# internal/depth, zona merah yang hanya boleh ditulis Al sendiri.
#
# Kenapa hook, bukan cukup permissions. .claude/settings.json sudah men-deny
# Edit dan Write pada internal/depth/**, tetapi Bash tidak tersentuh aturan itu.
# `sed -i internal/depth/x.go` melewati kunci itu sepenuhnya. Temuan P2-6 pada
# docs/internal/audit-2026-08-20.md.
#
# Yang TETAP boleh, karena zona merah bukan zona rahasia:
#   cat internal/depth/CLAUDE.md
#   ls internal/depth/
#   go test ./internal/depth/ -run TestX -v
#   grep -rn ComputeDepth internal/depth/
#   go test ./internal/depth/ 2>&1 | tail -5      (redirect BUKAN ke zona merah)
#
# Yang ditolak:
#   sed -i ... internal/depth/depth.go
#   echo x > internal/depth/depth.go
#   cp /tmp/a.go internal/depth/
#   gofmt -w internal/depth/
#   python3 - <<PY ... internal/depth ... PY
#
# Ini pengaman, bukan sandbox. Ia menutup jalan yang tidak sengaja, bukan jalan
# yang sengaja dicari. Tujuannya mengingatkan, dan pengingat yang menolak lebih
# berguna daripada pengingat yang hanya ditulis di dokumen.

set -uo pipefail

masukan=$(cat)

# Ambil perintahnya. Tanpa jq, hook ini harus tetap aman, jadi kalau jq tidak
# ada kita membiarkan perintahnya jalan dan berkata jujur tentang itu.
if ! command -v jq >/dev/null 2>&1; then
  exit 0
fi

perintah=$(printf '%s' "$masukan" | jq -r '.tool_input.command // empty')

# Tidak menyentuh zona merah sama sekali, tidak ada yang perlu diperiksa.
if ! printf '%s' "$perintah" | grep -q 'internal/depth'; then
  exit 0
fi

tolak() {
  jq -nc --arg alasan "$1" '{
    hookSpecificOutput: {
      hookEventName: "PreToolUse",
      permissionDecision: "deny",
      permissionDecisionReason: $alasan
    }
  }'
  exit 0
}

pesan="internal/depth adalah ZONA MERAH. Al yang menulis di sana, bukan kamu.
Perintah ini ditolak karena akan mengubah berkas di zona itu, dan Bash adalah
jalan yang tidak tertutup oleh deny Edit dan Write di .claude/settings.json.

Yang boleh kamu lakukan di sana: membaca, menjalankan test, menunjukkan edge
case yang belum tertangani, dan bertanya. Tawarkan /teach untuk konsepnya atau
/review-mine setelah Al menulisnya. Lihat internal/depth/CLAUDE.md."

# 1. Redirect yang sasarannya berada di zona merah. Diperiksa terpisah supaya
#    `go test ./internal/depth/ 2>&1 | tail` tetap lolos: di situ ada tanda >
#    tetapi sasarannya bukan berkas zona merah.
if printf '%s' "$perintah" | grep -Eq '>>?[[:space:]]*"?'"'"'?[^|;&<>]*internal/depth'; then
  tolak "$pesan

Terdeteksi: pengalihan keluaran ke dalam internal/depth."
fi

# 2. Perintah yang memang bertugas mengubah berkas, dan menyebut zona merah.
pola_ubah='(\bsed\b[^|;]*-i|\bperl\b[^|;]*-[a-zA-Z]*i|\btee\b|\bcp\b|\bmv\b|\brm\b|\bln\b|\binstall\b|\btruncate\b|\bdd\b|\bpatch\b|\btouch\b|\bmkdir\b|git[[:space:]]+(apply|checkout|restore|stash|rm|mv)|\b(gofmt|goimports)\b[^|;]*-w|\bpython3?\b|\bnode\b|\bruby\b|\bperl\b|\bawk\b[^|;]*-i)'
if printf '%s' "$perintah" | grep -Eq "$pola_ubah"; then
  tolak "$pesan

Terdeteksi: perintah pengubah berkas yang menyebut internal/depth."
fi

# Menyebut zona merah tetapi tampaknya hanya membaca. Dibiarkan.
exit 0
