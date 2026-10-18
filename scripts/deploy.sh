#!/usr/bin/env bash
# Build binary, pasang ke INSTALL_DIR, restart service, lalu cek /healthz.
# config.json dan folder data TIDAK disentuh (keduanya harus di luar workspace runner).
#
# Variabel lingkungan (semua punya default):
#   INSTALL_DIR  lokasi binary           (default: /opt/transfer-checklist)
#   RESTART_CMD  perintah restart service (default: sudo systemctl restart transfer-checklist)
#   HEALTH_URL   URL health check         (default: http://127.0.0.1:8080/healthz)
set -euo pipefail

INSTALL_DIR="${INSTALL_DIR:-/opt/transfer-checklist}"
RESTART_CMD="${RESTART_CMD:-sudo systemctl restart transfer-checklist}"
HEALTH_URL="${HEALTH_URL:-http://127.0.0.1:8080/healthz}"

cd "$(dirname "$0")/.."

echo "==> build"
mkdir -p dist
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o dist/transfer-checklist ./cmd/server

echo "==> pasang ke ${INSTALL_DIR}"
mkdir -p "${INSTALL_DIR}"
# salin ke nama sementara lalu rename agar penggantian binary atomik
install -m 0755 dist/transfer-checklist "${INSTALL_DIR}/transfer-checklist.new"
if [ -f "${INSTALL_DIR}/transfer-checklist" ]; then
  cp -p "${INSTALL_DIR}/transfer-checklist" "${INSTALL_DIR}/transfer-checklist.prev"
fi
mv -f "${INSTALL_DIR}/transfer-checklist.new" "${INSTALL_DIR}/transfer-checklist"

echo "==> restart: ${RESTART_CMD}"
bash -c "${RESTART_CMD}"

echo "==> health check ${HEALTH_URL}"
for i in $(seq 1 15); do
  if curl -fsS --max-time 3 "${HEALTH_URL}" >/dev/null 2>&1; then
    echo "OK: service sehat (percobaan ${i})"
    exit 0
  fi
  sleep 1
done

echo "GAGAL: health check tidak lolos." >&2
if [ -f "${INSTALL_DIR}/transfer-checklist.prev" ]; then
  echo "Versi sebelumnya tersimpan di ${INSTALL_DIR}/transfer-checklist.prev (salin balik untuk rollback)." >&2
fi
exit 1
