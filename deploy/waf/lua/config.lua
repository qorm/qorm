-- config.lua — WAF gateway configuration. Tune here; every module reads this.
-- Two global modes: "block" enforces, "detect" only logs (safe roll-out).
local _M = {}

_M.enabled = true
_M.mode    = "block"          -- "block" | "detect"

-- Trusted sources skip ALL checks (health checkers, your own infra). CIDR.
_M.allow = {
  "127.0.0.1/32",
  "::1/128",
}

-- Always-deny sources (CIDR). Evaluated before everything else.
_M.deny = {
}

-- Geo blocking (needs a country resolver, e.g. ngx.var.geoip2_country set by
-- the geoip2 module, or an X-Country header from an upstream CDN). ISO codes.
_M.geo = {
  enabled   = false,
  var       = "geoip2_country", -- ngx.var name holding the 2-letter country
  deny      = {},               -- e.g. { CN = true, RU = true }
  allow     = nil,              -- if set, ONLY these are allowed
}

-- Per-IP request rate (sliding window). Beyond `limit`+`burst` -> 429.
_M.rate = {
  enabled = true,
  window  = 1,                  -- seconds
  limit   = 20,                 -- steady requests/sec/IP
  burst   = 40,                 -- short burst allowance
}

-- DDoS / CC: sustained flood or too many concurrent connections -> ban the IP.
_M.ddos = {
  enabled     = true,
  window      = 10,             -- observation window (s)
  threshold   = 300,            -- requests/window/IP over which we ban
  conn_limit  = 100,            -- max concurrent in-flight requests per IP
  ban_seconds = 600,            -- base ban duration
  escalate    = true,           -- repeat offenders get exponentially longer bans
  max_ban     = 86400,          -- cap on escalated bans (s)
}

-- Request shape constraints.
_M.limits = {
  max_uri     = 3072,
  max_args    = 4096,
  max_body    = 2 * 1024 * 1024, -- 2 MB (Content-Length based)
  max_headers = 80,
  methods     = { GET=true, POST=true, HEAD=true, PUT=true, DELETE=true, PATCH=true, OPTIONS=true },
}

-- Known scanners / attack tools (lowercased substrings matched in User-Agent).
_M.bad_ua = {
  "sqlmap", "nikto", "nmap", "masscan", "zgrab", "acunetix", "nessus",
  "openvas", "dirbuster", "gobuster", "wpscan", "havij", "fimap", "w3af",
  "hydra", "medusa", "arachni", "nuclei", "xsser", "commix",
}
_M.block_empty_ua = false        -- some legit clients omit UA; off by default

-- Signature engine toggles (see rules.lua).
_M.rules = {
  enabled       = true,
  sqli          = true,
  xss           = true,
  traversal     = true,
  rce           = true,          -- command injection
  scan_paths    = true,          -- probes for .env, wp-admin, .git, etc.
  inspect_body  = true,          -- scan request body (bounded by max_body)
  ban_on_hit    = true,          -- a signature hit also bans the IP
}

return _M
