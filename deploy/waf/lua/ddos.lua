-- ddos.lua — DDoS / CC mitigation. Two signals per IP:
--   1. too many CONCURRENT in-flight requests (slow-loris / connection flood)
--   2. sustained REQUEST rate over a wide window (volumetric flood)
-- Either trips a dynamic ban (escalating). Concurrency is a slot taken in the
-- access phase (enter) and released in the log phase (leave).
local cfg       = require "config"
local util      = require "util"
local blocklist = require "blocklist"
local _M        = {}

local function counters() return ngx.shared.waf_counters end
local function conns()    return ngx.shared.waf_conn end

-- enter counts the request, takes a concurrency slot, and bans on breach.
-- Returns nil when ok, or (403, reason) when the IP was just banned.
function _M.enter(ip)
  local d = cfg.ddos
  if not d.enabled then return nil end

  -- concurrency slot (always release it in leave)
  local live = conns():incr(ip, 1, 0, 60) or 1
  ngx.ctx.waf_conn_held = true
  if live > d.conn_limit then
    local dur = blocklist.ban(ip, d.ban_seconds, "conn-flood")
    return 403, "ddos-conn:" .. live .. " ban=" .. math.floor(dur)
  end

  -- sustained volumetric flood over the wide window
  local bucket = math.floor(ngx.now() / d.window)
  local reqs   = util.incr(counters(), "dd:" .. ip .. ":" .. bucket, d.window + 1)
  if reqs > d.threshold then
    local dur = blocklist.ban(ip, d.ban_seconds, "flood")
    return 403, "ddos-flood:" .. reqs .. "/" .. d.window .. "s ban=" .. math.floor(dur)
  end
  return nil
end

-- leave releases this request's concurrency slot (log phase).
function _M.leave(ip)
  if not ngx.ctx.waf_conn_held then return end
  local n = conns():incr(ip, -1, 0, 60)
  if n and n < 0 then conns():set(ip, 0, 60) end
end

return _M
