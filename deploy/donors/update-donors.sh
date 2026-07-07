#!/usr/bin/env sh
# Daily donors refresh (cron alternative to the systemd timer).
# Load creds from a non-committed env file, then write the site's donors.json.
set -e
. /etc/qorm/donors.env    # export PATREON_ACCESS_TOKEN (+ optional PATREON_CAMPAIGN_ID)
exec qorm-donors -o /srv/qorm-site/donors.json -months 24
