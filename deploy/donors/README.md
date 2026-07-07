# Supporters list (auto-refreshed from Patreon)

`qorm-donors` reads the campaign's patrons via the Patreon API v2, formats a
privacy-friendly name + lifetime support, and writes the `donors.json` the
website renders. A daily timer keeps it current. No credentials are ever committed.

## One-time setup

1. **Register a client** at https://www.patreon.com/portal/registration/register-clients
   and copy its **Creator's Access Token** (it acts on your own campaign — no OAuth
   dance needed).
2. **Store it** outside git:
   ```sh
   sudo install -d -m 700 /etc/qorm
   sudo cp deploy/donors/donors.env.example /etc/qorm/donors.env
   sudo chmod 600 /etc/qorm/donors.env
   sudo $EDITOR /etc/qorm/donors.env        # PATREON_ACCESS_TOKEN=…
   ```
3. **Install the tool:**
   ```sh
   go build -o /usr/local/bin/qorm-donors ./cmd/qorm-donors
   ```

## Daily refresh

### systemd (recommended)
```sh
sudo cp deploy/donors/qorm-donors.service deploy/donors/qorm-donors.timer /etc/systemd/system/
sudo systemctl enable --now qorm-donors.timer
systemctl start qorm-donors.service     # run once now
```
Writes `/srv/qorm-site/donors.json` (adjust the paths in the unit to match the site
root). The timer fires daily at 04:15.

### cron
```cron
15 4 * * *  /path/to/deploy/donors/update-donors.sh
```

## What it writes

```json
{ "updated": "…", "count": 3, "total": 21,
  "donors": [ { "name": "Ada L.", "total": 10, "currency": "USD" } ] }
```

- Names are privacy-friendly: **first name + last initial**. Only patrons with
  lifetime support > 0 are listed; amount is lifetime support.
- If the token is missing or the API errors, the tool exits non-zero and **leaves
  the existing `donors.json` untouched** — a bad run never blanks the list.

## Manual run

```sh
export PATREON_ACCESS_TOKEN=…
qorm-donors -o site/donors.json
```
