--!nonstrict
-- ULT Control service - the only entry that talks to the headphones.
-- It runs the ultcontrol CLI one command at a time and publishes the result
-- through noctalia.state; the bar widget and panel are thin clients.
--
--   ult.status  JSON from `ultcontrol --json status`
--   ult.busy    "1" while a command runs
--   ult.error   last error text, "" when fine
--   ult.cmd     "<n>|<args...>" written by the UI, e.g. "3|mode nc 20"

local queue = {}
local running = false
local counter = 0

local function bin()
  local c = noctalia.getConfig("command")
  if c == nil or c == "" then return "ultcontrol" end
  return c
end

local function pump()
  if running or #queue == 0 then return end
  running = true
  noctalia.state.set("ult.busy", "1")
  local args = table.remove(queue, 1)
  local argv = { bin(), "--json" }
  for word in string.gmatch(args, "%S+") do
    table.insert(argv, word)
  end
  noctalia.runAsync(argv, function (res)
    running = false
    noctalia.state.set("ult.busy", #queue > 0 and "1" or "0")
    local ok = res.exitCode == 0
    if ok then
      local line = string.match(res.stdout, "(%b{})")
      if line then
        noctalia.state.set("ult.status", line)
        noctalia.state.set("ult.error", "")
      else
        noctalia.state.set("ult.error", "bad output")
      end
    else
      local msg = res.stderr ~= "" and res.stderr or res.stdout
      noctalia.state.set("ult.error", string.sub(msg, 1, 160))
    end
    pump()
  end, 30000)
end

-- Only these subcommands may be run; ult.cmd is shared state, so don't trust it blindly.
local ALLOWED = { status = true, mode = true, eq = true }

local function enqueue(args)
  local first = string.match(args, "^(%S+)")
  if not first or not ALLOWED[first] then
    noctalia.state.set("ult.error", "command not allowed")
    return
  end
  table.insert(queue, args)
  pump()
end

noctalia.state.set("ult.status", "")
noctalia.state.set("ult.busy", "0")
noctalia.state.set("ult.error", "")

noctalia.state.watch("ult.cmd", function (value)
  local args = string.match(value or "", "^%d+|(.*)$")
  if args and args ~= "" then enqueue(args) end
end)

local elapsed = 0
noctalia.setUpdateInterval(10000)
function update()
  local every = noctalia.getConfig("poll_seconds") or 120
  if elapsed == 0 or elapsed >= every then
    elapsed = 0
    -- skip polling if a command is pending; it will return fresh status anyway
    if not running and #queue == 0 then enqueue("status") end
  end
  elapsed = elapsed + 10
end
