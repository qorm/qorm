-- rules.lua — signature engine. A curated, low-false-positive ruleset (a
-- ModSecurity-CRS-lite) matched with PCRE over the decoded request surfaces:
-- URI, query args, notable headers, cookies, and (bounded) the request body.
local cfg = require "config"
local _M  = {}

-- Each rule: g = group toggle (config.rules[g]), id, msg, re = PCRE.
-- Flags "ijo": case-insensitive, ignore-whitespace-in-pattern off, compile-once.
local RULES = {
  -- SQL injection
  { g = "sqli", id = "sqli-union",   msg = "SQL union",        re = [=[union[\s/*!]+select]=] },
  { g = "sqli", id = "sqli-bool",    msg = "SQL tautology",    re = [=[\b(or|and)\b\s+['"\d][\w\s]*=\s*['"\d]]=] },
  { g = "sqli", id = "sqli-time",    msg = "SQL time-based",   re = [=[(sleep|benchmark|pg_sleep|waitfor\s+delay)\s*\(]=] },
  { g = "sqli", id = "sqli-meta",    msg = "SQL metadata",     re = [=[information_schema|\bfrom\s+mysql\.]=] },
  { g = "sqli", id = "sqli-stack",   msg = "SQL stacked/DDL",  re = [=[;\s*(drop|alter|create|truncate|insert|update|delete)\s]=] },
  -- Cross-site scripting
  { g = "xss",  id = "xss-script",   msg = "XSS <script>",     re = [=[<\s*script\b]=] },
  { g = "xss",  id = "xss-event",    msg = "XSS event handler",re = [=[on(error|load|mouseover|click|focus|toggle|animationstart)\s*=]=] },
  { g = "xss",  id = "xss-proto",    msg = "XSS js/data URI",  re = [=[(javascript|vbscript)\s*:|data:text/html]=] },
  { g = "xss",  id = "xss-tag",      msg = "XSS injected tag", re = [=[<\s*(img|svg|iframe|body|object|embed|details)\b[^>]*(on\w+\s*=|src\s*=)]=] },
  -- Path traversal / local file
  { g = "traversal", id = "lfi-dotdot", msg = "path traversal", re = [=[(\.\./|\.\.\\|%2e%2e[/\\%]|%252e)]=] },
  { g = "traversal", id = "lfi-file",   msg = "sensitive file", re = [=[/etc/(passwd|shadow)\b|/proc/self/environ|boot\.ini|windows/win\.ini]=] },
  -- Command injection
  { g = "rce",  id = "rce-shell",    msg = "command injection",re = [=[(;|\||`|\$\()\s*(cat|ls|id|whoami|wget|curl|nc|bash|sh|python|perl|powershell)\b]=] },
  { g = "rce",  id = "rce-subst",    msg = "shell substitution",re = [=[\$\([^)]{0,64}\)|`[^`]{0,64}`]=] },
  -- Scanner / probe paths
  { g = "scan_paths", id = "scan-dotfiles", msg = "dotfile probe", re = [=[/(\.env\b|\.git/|\.aws/|\.ssh/|\.htpasswd|\.svn/|\.DS_Store)]=] },
  { g = "scan_paths", id = "scan-admin",    msg = "admin probe",   re = [=[/(wp-admin|wp-login|phpmyadmin|administrator/|\.well-known/security)]=] },
  { g = "scan_paths", id = "scan-shell",    msg = "webshell probe",re = [=[/(shell|cmd|eval|c99|r57|webshell)\.(php|jsp|asp|aspx)]=] },
}

-- scan runs the enabled rules over one string. Returns (id, msg) on a hit.
function _M.scan(str, source)
  if not str or str == "" then return nil end
  for i = 1, #RULES do
    local r = RULES[i]
    if cfg.rules[r.g] and ngx.re.find(str, r.re, "ijo") then
      return r.id, r.msg .. " in " .. source
    end
  end
  return nil
end

-- check inspects the whole request. Returns nil when clean, or (403, id, msg).
function _M.check()
  if not cfg.rules.enabled then return nil end

  -- URI (attackers URL-encode payloads, so scan the decoded form too)
  local uri = ngx.var.request_uri or ""
  local id, msg = _M.scan(uri, "uri")
  if not id then id, msg = _M.scan(ngx.unescape_uri(uri), "uri") end
  if id then return 403, id, msg end

  -- notable headers + cookies
  local h = ngx.req.get_headers()
  for _, name in ipairs({ "referer", "user-agent", "cookie", "x-forwarded-for" }) do
    local v = h[name]
    if type(v) == "string" then
      id, msg = _M.scan(ngx.unescape_uri(v), name)
      if id then return 403, id, msg end
    end
  end

  -- request body (only what's buffered in memory, bounded by client_body_buffer_size)
  if cfg.rules.inspect_body then
    local m = ngx.req.get_method()
    if m == "POST" or m == "PUT" or m == "PATCH" then
      ngx.req.read_body()
      local body = ngx.req.get_body_data()
      if body then
        id, msg = _M.scan(ngx.unescape_uri(body), "body")
        if id then return 403, id, msg end
      end
    end
  end
  return nil
end

return _M
