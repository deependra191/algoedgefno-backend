# Kite Live Probe

This is local-only read-only R&D tooling for the Zerodha backfill runbook. It
does not import storage, does not connect to Postgres, and does not insert data.

## Create the Kite app

1. Open the Kite Connect developer console.
2. Create a Connect app with the historical data add-on enabled.
3. Set the redirect URL to a local URL you control, for example
   `http://127.0.0.1:8080/kite/callback`.
4. Copy the app's API key and API secret into your local shell only.

## Get an access token

Export the API key and secret:

```bash
export KITE_API_KEY='your_api_key'
export KITE_API_SECRET='your_api_secret'
```

Open the login URL:

```bash
printf 'https://kite.zerodha.com/connect/login?v=3&api_key=%s\n' "$KITE_API_KEY"
```

After login, Zerodha redirects to your registered redirect URL with a
`request_token` query parameter. Copy only that request token.

Exchange it for an access token:

```bash
read -rsp 'Kite request_token: ' REQUEST_TOKEN; echo
go run ./local-rnd/kite-token -request-token "$REQUEST_TOKEN" -out /private/tmp/kite-access-token.env
unset REQUEST_TOKEN
```

Source the generated env file in your local shell:

```bash
source /private/tmp/kite-access-token.env
```

Do not commit these values, paste them into chat, or include them in logs. The
access token expires at about 06:00 IST the next day.

## Run the probe

```bash
go run ./local-rnd/kite-probe
```

Useful overrides:

```bash
go run ./local-rnd/kite-probe \
  -underlying NIFTY \
  -index-symbol 'NIFTY 50' \
  -expired-from 2024-01-01 \
  -expired-to 2024-01-05 \
  -roll-from 2024-12-20 \
  -roll-to 2025-01-05 \
  -burst 6
```

If auto-resolution fails because Kite changes symbols, pass explicit instrument
tokens:

```bash
go run ./local-rnd/kite-probe -index-token 256265 -future-token '<active_future_token>'
```

Paste the probe output back into the task thread, but never paste env values,
the access token, the API secret, or the Authorization header.
