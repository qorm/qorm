-- waf.lua — the orchestrator. Runs the checks in cheap-to-expensive order in the
-- access phase, releases DDoS concurrency in the log phase, and exposes a small
-- status endpoint. Wire it from nginx: access_by_lua_block / log_by_lua_block.
local cfg       = require "config"
local util      = require "util"
local ipfilter  = require "ipfilter"
local blocklist = require "blocklist"
local bots      = require "bots"
local ratelimit = require "ratelimit"
local ddos      = require "ddos"
local rules     = require "rules"
local log       = require "log"
local _M        = {}

-- CIDRs of proxies you trust to set X-Forwarded-For (your CDN / load balancer).
-- Leave as loopback only if the WAF is the edge.
_M.trusted_proxies = { "127.0.0.1/32", "::1/128" }

local function reject(ip, status, rule, detail)
  if cfg.mode == "detect" then
    log.detect(ip, status, rule, detail)
    return
  end
  log.block(ip, status, rule, detail)
  ngx.status = status
  ngx.header["Content-Type"] = "text/plain; charset=utf-8"
  ngx.header["X-WAF-Block"] = rule
  ngx.say("Request blocked by WAF (", rule, ")")
  return ngx.exit(status)
end

-- access is the gate. Return without exiting = allow through to the upstream.
function _M.access()
  if not cfg.enabled then return end
  local ip = util.client_ip(_M.trusted_proxies)
  ngx.ctx.waf_ip = ip

  if ipfilter.allowed(ip) then return end             -- trusted: skip all

  if blocklist.is_banned(ip) then                     -- already banned
    return reject(ip, 403, "banned")
  end

  local st, why = ipfilter.check(ip)                  -- static ip / geo
  if st then return reject(ip, st, why) end

  st, why = bots.check(ip)                             -- request shape + UA
  if st then return reject(ip, st, why) end

  st, why = ddos.enter(ip)                             -- ddos / cc (+ concurrency)
  if st then return reject(ip, st, why) end

  local retry
  st, why, retry = ratelimit.check(ip)                -- steady-rate limit
  if st then
    if retry then ngx.header["Retry-After"] = retry end
    return reject(ip, st, why)
  end

  local id, msg                                       -- signature engine
  st, id, msg = rules.check()
  if st then
    if cfg.rules.ban_on_hit then blocklist.ban(ip, cfg.ddos.ban_seconds, id) end
    return reject(ip, st, id, msg)
  end
end

-- finish releases the concurrency slot. Call from log_by_lua_block.
function _M.finish()
  ddos.leave(ngx.ctx.waf_ip)
end

-- status renders a tiny JSON metrics + ban summary (content_by_lua on an
-- internal, access-restricted location).
function _M.status()
  local m  = ngx.shared.waf_metrics
  local bl = ngx.shared.waf_bans
  ngx.header["Content-Type"] = "application/json"
  local keys = m:get_keys(200)
  local parts = { '"mode":"' .. cfg.mode .. '"', '"enabled":' .. tostring(cfg.enabled) }
  for _, k in ipairs(keys) do
    parts[#parts + 1] = '"' .. k .. '":' .. (m:get(k) or 0)
  end
  parts[#parts + 1] = '"active_bans":' .. #bl:get_keys(1024)
  ngx.say("{", table.concat(parts, ","), "}")
end

return _M
