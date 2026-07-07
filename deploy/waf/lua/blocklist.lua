-- blocklist.lua — dynamic IP bans backed by a shared dict, with escalation.
-- A ban records an expiry; repeat offenders (tracked by a strike counter) get
-- exponentially longer bans up to config.ddos.max_ban.
local cfg = require "config"
local _M  = {}

local function bans()    return ngx.shared.waf_bans end
local function strikes() return ngx.shared.waf_strikes end

-- is_banned reports whether ip is currently banned (and for how long left).
function _M.is_banned(ip)
  local until_ts = bans():get(ip)
  if not until_ts then return false end
  local left = until_ts - ngx.now()
  if left <= 0 then
    bans():delete(ip)
    return false
  end
  return true, left
end

-- ban blocks ip for base seconds, escalating on repeat strikes.
function _M.ban(ip, base, reason)
  base = base or cfg.ddos.ban_seconds
  local dur = base
  if cfg.ddos.escalate then
    local n = strikes():incr(ip, 1, 0, cfg.ddos.max_ban) or 1
    -- 1st: base, 2nd: base*2, 3rd: base*4 ... capped
    dur = math.min(base * (2 ^ (n - 1)), cfg.ddos.max_ban)
  end
  bans():set(ip, ngx.now() + dur, dur)
  return dur, reason
end

-- unban clears a ban + strikes (for an admin/allowlist path).
function _M.unban(ip)
  bans():delete(ip)
  strikes():delete(ip)
end

return _M
