-- util.lua — shared helpers: client IP, IPv4 CIDR matching, shared-dict counters.
local bit = require "bit"
local _M  = {}

local band, lshift = bit.band, bit.lshift

-- ipv4_to_int converts "a.b.c.d" to a 32-bit integer, or nil if not IPv4.
function _M.ipv4_to_int(ip)
  local a, b, c, d = ip:match("^(%d+)%.(%d+)%.(%d+)%.(%d+)$")
  if not a then return nil end
  a, b, c, d = tonumber(a), tonumber(b), tonumber(c), tonumber(d)
  if a > 255 or b > 255 or c > 255 or d > 255 then return nil end
  -- lshift(a,24) can overflow LuaJIT's signed int; use multiplication.
  return a * 16777216 + b * 65536 + c * 256 + d
end

-- cidr_match reports whether an IPv4 address falls in "net/bits" (or an exact
-- "net"). IPv6 is matched only by exact string equality here.
function _M.cidr_match(ip, cidr)
  local net, bits = cidr:match("^([^/]+)/(%d+)$")
  if not net then
    return ip == cidr           -- bare address (incl. IPv6)
  end
  local ipn  = _M.ipv4_to_int(ip)
  local netn = _M.ipv4_to_int(net)
  if not ipn or not netn then return false end
  bits = tonumber(bits)
  if bits <= 0 then return true end
  if bits >= 32 then return ipn == netn end
  -- mask = top `bits` bits set
  local mask = (2 ^ 32) - (2 ^ (32 - bits))
  return band(ipn, mask) == band(netn, mask)
end

-- in_cidr_list reports whether ip matches any CIDR in the list.
function _M.in_cidr_list(ip, list)
  if not list then return false end
  for i = 1, #list do
    if _M.cidr_match(ip, list[i]) then return true end
  end
  return false
end

-- client_ip resolves the real client address, honouring a trusted
-- X-Forwarded-For ONLY when the immediate peer is a trusted proxy CIDR.
function _M.client_ip(trusted_proxies)
  local peer = ngx.var.remote_addr
  if trusted_proxies and _M.in_cidr_list(peer, trusted_proxies) then
    local xff = ngx.var.http_x_forwarded_for
    if xff then
      -- left-most is the origin client
      local first = xff:match("^%s*([^,%s]+)")
      if first then return first end
    end
  end
  return peer
end

-- incr bumps a counter in a shared dict, initialising with a TTL. Returns the
-- new value. Safe under contention.
function _M.incr(dict, key, ttl)
  local newval, err = dict:incr(key, 1, 0, ttl)
  if not newval then
    -- key may have expired between ops; try to (re)create it
    dict:set(key, 1, ttl)
    return 1
  end
  return newval
end

return _M
