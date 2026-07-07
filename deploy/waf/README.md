# QORM WAF Gateway (OpenResty)

A general-purpose reverse proxy + web application firewall built on OpenResty
(nginx + LuaJIT). Put it in front of any backend — the QORM site, a packaged
`qorm package -p web` app, or an unrelated service — and it filters hostile
traffic before it reaches the origin.

## What it stops

| Layer | Mechanism | Config |
|---|---|---|
| Volumetric flood / CC | per-IP request rate over a window → escalating ban | `config.ddos` |
| Slow-loris / conn flood | per-IP concurrent-request cap → ban | `config.ddos.conn_limit` |
| Burst abuse | sliding-window rate limit → 429 | `config.rate` |
| SQLi / XSS / traversal / RCE | curated PCRE signatures over URI, args, headers, cookies, body | `config.rules` |
| Scanners / probes | bad-UA list + `.env`/`.git`/`wp-admin` path signatures | `config.bad_ua`, `rules` |
| IP / geo | static allow/deny CIDR + optional country filter | `config.allow/deny/geo` |
| Malformed requests | method / URI / arg / header-count / body-size caps | `config.limits` |

Defence in depth: nginx's own `limit_req` / `limit_conn` zones and tight
timeouts (L0, in `nginx.conf`) blunt floods before the Lua WAF (L1) even runs.

## Layout

```
deploy/waf/
  nginx.conf          main config: shared dicts, L0 zones, upstream, includes
  conf.d/gateway.conf the protected server block (proxy + WAF hooks)
  lua/
    config.lua        all tunables (thresholds, lists, toggles)
    waf.lua           orchestrator: access_by_lua gate + log_by_lua release
    util.lua          client IP, IPv4 CIDR match, shared-dict counters
    ipfilter.lua      static allow/deny + geo
    blocklist.lua     dynamic bans with escalation (shared dict)
    ratelimit.lua     per-IP sliding-window rate limit
    ddos.lua          flood + concurrency detection → ban
    bots.lua          request-shape constraints + bad UA
    rules.lua         SQLi/XSS/traversal/RCE/scan signatures
    log.lua           structured key=value logging + metrics
  test/
    unit.lua          pure-Lua CIDR tests (luajit test/unit.lua)
    attack.sh         end-to-end: boots the gateway, fires payloads, asserts
```

## Run

```sh
# 1. point the upstream at your backend in nginx.conf (upstream backend { ... })
# 2. start it
openresty -p "$PWD/deploy/waf" -c nginx.conf
# 3. metrics (from localhost)
curl -s localhost/waf-status
```

## Roll out safely

Set `config.mode = "detect"` first: the WAF logs what it *would* block
(`[WAF] DETECT …` in the error log) without rejecting, so you can tune the rules
and thresholds against real traffic. Flip to `"block"` once the false-positive
rate is acceptable.

## Tuning notes

- `trusted_proxies` in `waf.lua` must list your CDN/LB CIDRs, or `X-Forwarded-For`
  is ignored and every client looks like the proxy.
- Body inspection only sees bodies buffered in memory
  (`client_body_buffer_size`); larger uploads spill to disk and are skipped by
  the signature engine (size is still capped by `client_max_body_size`).
- Geo needs a country source — the `geoip2` module setting `$geoip2_country`, or
  a trusted `X-Country` header from your CDN; wire it via `config.geo.var`.
- Bans live in shared memory: a full restart clears them; a reload preserves the
  dicts.

## Test

```sh
luajit deploy/waf/test/unit.lua      # CIDR logic, no OpenResty needed
deploy/waf/test/attack.sh            # full gateway (needs openresty)
```
