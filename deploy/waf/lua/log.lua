-- log.lua — structured WAF event logging to the nginx error log. Parseable
-- key=value form so it can be shipped to a SIEM. Also bumps a shared-dict
-- metric per rule for a quick /waf-status readout.
local _M = {}

local function metrics() return ngx.shared.waf_metrics end

local function emit(level, kind, ip, status, rule, detail)
  metrics():incr("hits", 1, 0)
  metrics():incr("rule:" .. rule, 1, 0)
  ngx.log(level, "[WAF] ", kind,
    " ip=", ip or "-",
    " status=", status or "-",
    " rule=", rule or "-",
    " method=", ngx.req.get_method(),
    " host=", ngx.var.host or "-",
    " uri=", ngx.var.request_uri or "-",
    detail and (" detail=\"" .. detail .. "\"") or "")
end

function _M.block(ip, status, rule, detail)
  emit(ngx.WARN, "BLOCK", ip, status, rule, detail)
end

function _M.detect(ip, status, rule, detail)
  emit(ngx.NOTICE, "DETECT", ip, status, rule, detail)
end

return _M
