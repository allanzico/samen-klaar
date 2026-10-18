#!/usr/bin/env bash
# Checks whether the PayPal sandbox platform app may vault a PayPal wallet (see docs/AGENT_PAYMENT_HOLDS.md).
# Sandbox only. Reads PAYPAL_* from the environment or the ignored .env (ENV_FILE=path to override). Prints no credentials.
set -euo pipefail
cd "$(dirname "$0")/.."
env_file=${ENV_FILE:-.env}
if [ -f "$env_file" ]; then set -a; . "$env_file"; set +a; fi
[ "${PAYPAL_ENV:-}" = sandbox ] || { echo "PAYPAL_ENV must be sandbox"; exit 1; }
base=https://api-m.sandbox.paypal.com
token=$(curl -s -m 30 -u "$PAYPAL_CLIENT_ID:$PAYPAL_CLIENT_SECRET" -d grant_type=client_credentials "$base/v1/oauth2/token" |
  python3 -I -c 'import sys,json; print(json.load(sys.stdin).get("access_token",""))')
[ -n "$token" ] || { echo "could not get a sandbox access token"; exit 1; }
call() { # method path body
  curl -s -m 30 -w '\nHTTP %{http_code}\n' -X "$1" "$base$2" \
    -H "Authorization: Bearer $token" -H 'Content-Type: application/json' \
    -H "PayPal-Request-Id: probe-$RANDOM-$(date +%s)" \
    -H "PayPal-Partner-Attribution-Id: ${PAYPAL_PARTNER_ATTRIBUTION_ID:-}" -d "$3" | tail -c 700
  echo
}
echo "== setup token (expect 200/201 PAYER_ACTION_REQUIRED when vaulting is enabled)"
call POST /v3/vault/setup-tokens '{"payment_source":{"paypal":{"description":"SamenKlaar vault probe","usage_pattern":"IMMEDIATE","usage_type":"PLATFORM","customer_type":"CONSUMER","experience_context":{"return_url":"https://example.com/r","cancel_url":"https://example.com/c"}}}}'
echo "== authorize order that stores the payer in the vault"
call POST /v2/checkout/orders '{"intent":"AUTHORIZE","purchase_units":[{"amount":{"currency_code":"EUR","value":"10.00"}}],"payment_source":{"paypal":{"attributes":{"vault":{"store_in_vault":"ON_SUCCESS","usage_type":"PLATFORM","customer_type":"CONSUMER"}},"experience_context":{"return_url":"https://example.com/r","cancel_url":"https://example.com/c"}}}}'
