-- ipfilter.lua — static allow/deny (CIDR) and geo filtering.
local cfg  = require "config"
local util = require "util"
local _M   = {}

-- allowed reports whether ip is on the trusted allowlist (skip all checks).
function _M.allowed(ip)
  return util.in_cidr_list(ip, cfg.allow)
end

-- check returns nil when ok, or (status, reason) when the request must be
-- rejected on IP/geo grounds.
function _M.check(ip)
  if util.in_cidr_list(ip, cfg.deny) then
    return 403, "ip-deny"
  end
  if cfg.geo.enabled then
    local country = ngx.var[cfg.geo.var]
    if country and country ~= "" then
      if cfg.geo.allow then
        if not cfg.geo.allow[country] then return 403, "geo-not-allowed:" .. country end
      elseif cfg.geo.deny[country] then
        return 403, "geo-deny:" .. country
      end
    end
  end
  return nil
end

return _M
