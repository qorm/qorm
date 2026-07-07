# QORM website (qorm.com)

The official landing page. One self-contained `index.html` (inline CSS/JS, no
external requests, no build step), plus the brand mark. It embodies QORM's
dual-consumer story and is styled with Apple-default tokens.

- **Bilingual** — English / 中文, toggled in the nav (persisted, and it picks up
  a `zh-*` browser locale on first visit).
- **Themed** — light / dark, following the OS by default with a manual toggle.
- **No emoji** — every icon is inline SVG (`stroke:currentColor`), per house rules.
- **Donations** — via Patreon (see TERMS.md; four tiers: Community / Indie / Studio / Supporter).

## Preview locally

```sh
cd site && python3 -m http.server 8080   # then open http://localhost:8080
```

## Deploy behind the WAF

The site is static, so it sits directly behind the OpenResty WAF gateway
([`deploy/waf`](../deploy/waf)) — anti-DDoS, rate limiting, and the signature
ruleset apply before a byte is served.

```sh
# 1. copy the site to the server
rsync -a site/ user@host:/srv/qorm-site/

# 2. enable the static-site server block in deploy/waf/conf.d/gateway.conf
#    (root /srv/qorm-site), then start / reload the gateway
openresty -p /srv/qorm/deploy/waf -c nginx.conf
```

## Content source

Copy is written in-file. The doc/reference links point at the repo
(`docs/…`, `llms.txt`, `examples/…`), so the site stays a thin front door and the
repository remains the single source of truth.
