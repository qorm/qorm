-- unit.lua — pure-logic tests, runnable with plain luajit (no OpenResty needed).
-- Covers the CIDR math (the riskiest bit-twiddling). Run from deploy/waf:
--     luajit test/unit.lua
package.path = "./lua/?.lua;" .. package.path
local util = require "util"

local pass, fail = 0, 0
local function check(cond, name)
  if cond then pass = pass + 1 else fail = fail + 1; io.write("FAIL: ", name, "\n") end
end

check(util.ipv4_to_int("0.0.0.0") == 0, "ip 0.0.0.0")
check(util.ipv4_to_int("255.255.255.255") == 4294967295, "ip max")
check(util.ipv4_to_int("10.0.0.40") == 167772200, "ip 10.0.0.40")
check(util.ipv4_to_int("999.0.0.0") == nil, "reject invalid octet")
check(util.ipv4_to_int("not-an-ip") == nil, "reject non-ip")

check(util.cidr_match("10.0.0.40", "10.0.0.0/8") == true,  "in /8")
check(util.cidr_match("11.0.0.1",  "10.0.0.0/8") == false, "out of /8")
check(util.cidr_match("192.168.1.5", "192.168.1.0/24") == true,  "in /24")
check(util.cidr_match("192.168.2.5", "192.168.1.0/24") == false, "out of /24")
check(util.cidr_match("127.0.0.1", "127.0.0.1/32") == true,  "exact /32")
check(util.cidr_match("10.1.2.3",  "0.0.0.0/0") == true,     "match-all /0")
check(util.cidr_match("::1", "::1") == true,                 "ipv6 exact")

check(util.in_cidr_list("10.0.0.5", { "192.168.0.0/16", "10.0.0.0/8" }) == true,  "list hit")
check(util.in_cidr_list("8.8.8.8",  { "192.168.0.0/16", "10.0.0.0/8" }) == false, "list miss")

io.write(string.format("util: %d passed, %d failed\n", pass, fail))
os.exit(fail == 0 and 0 or 1)
