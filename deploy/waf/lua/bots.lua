-- bots.lua — request-shape constraints + malicious user-agent filtering.
local cfg = require "config"
local _M  = {}

-- check returns nil when ok, or (status, reason) when the request is malformed
-- or from a known bad client.
function _M.check(ip)
  local L = cfg.limits

  local method = ngx.req.get_method()
  if not L.methods[method] then return 405, "method:" .. method end

  local uri = ngx.var.request_uri or ""
  if #uri > L.max_uri then return 414, "uri-too-long:" .. #uri end

  local args = ngx.var.args
  if args and #args > L.max_args then return 400, "args-too-long" end

  local cl = tonumber(ngx.var.content_length)
  if cl and cl > L.max_body then return 413, "body-too-large:" .. cl end

  -- header count (cap the parse to avoid the "too many headers" truncation warn)
  local h = ngx.req.get_headers(L.max_headers + 10)
  local hc = 0
  for _ in pairs(h) do hc = hc + 1 end
  if hc > L.max_headers then return 400, "too-many-headers:" .. hc end

  -- user-agent screening
  local ua = h["user-agent"]
  if type(ua) ~= "string" or ua == "" then
    if cfg.block_empty_ua then return 403, "empty-ua" end
  else
    local low = ua:lower()
    for i = 1, #cfg.bad_ua do
      if low:find(cfg.bad_ua[i], 1, true) then
        return 403, "bad-ua:" .. cfg.bad_ua[i]
      end
    end
  end
  return nil
end

return _M
