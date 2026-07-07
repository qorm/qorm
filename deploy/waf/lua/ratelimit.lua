-- ratelimit.lua — per-IP sliding-window rate limiting (steady rate + burst).
-- Uses a shared-dict counter keyed by IP and the current time bucket.
local cfg  = require "config"
local util = require "util"
local _M   = {}

local function counters() return ngx.shared.waf_counters end

-- check returns nil when within budget, or (429, reason, retry_after) when the
-- caller must throttle.
function _M.check(ip)
  local r = cfg.rate
  if not r.enabled then return nil end
  local win    = r.window
  local bucket = math.floor(ngx.now() / win)
  local key    = "rl:" .. ip .. ":" .. bucket
  local n      = util.incr(counters(), key, win + 1)
  if n > (r.limit * win) + r.burst then
    return 429, "rate-limit", win
  end
  return nil
end

return _M
