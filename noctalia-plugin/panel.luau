--!nonstrict
-- ULT Control panel: NC/ambient, ambient level, EQ preset, ULT mode, bands.
-- Commands go to the service through the ult.cmd state key.

local PRESETS = { "Off", "Bright", "Excited", "Mellow", "Relaxed", "Vocal", "Treble", "Bass", "Speech" }
local BANDS = { "Clear Bass", "400 Hz", "1 kHz", "2.5 kHz", "6.3 kHz", "16 kHz" }

local status = nil
local busy = false
local err = ""
local seq = 0

local function send(args)
  seq = seq + 1
  noctalia.state.set("ult.cmd", tostring(seq) .. "|" .. args)
end

local function bandStep(i, delta)
  local g = {}
  for k = 1, 6 do g[k] = status.eq_bands[k] end
  g[i] = math.max(-10, math.min(10, g[i] + delta))
  send("eq custom " .. table.concat(g, " "))
end

local function modeButton(label, mode, arg)
  local active = status and status.mode == mode
  return ui.button({
    text = label,
    variant = active and "primary" or "secondary",
    flexGrow = 1,
    enabled = not busy,
    onClick = function () send("mode " .. mode .. (arg or "")) end,
  })
end

local function render()
  if not status then
    panel.render(ui.column({ padding = 16, gap = 8 }, {
      ui.label({ text = noctalia.tr("ui.title") }),
      ui.label({ text = err ~= "" and noctalia.tr("ui.offline") or noctalia.tr("ui.busy") }),
      ui.button({ text = noctalia.tr("ui.refresh"), onClick = function () send("status") end }),
    }))
    return
  end

  local level = status.ambient_level or 0
  local presetIdx = 1
  local presetLabels = {}
  for i, p in ipairs(PRESETS) do
    presetLabels[i] = p
    if p == status.eq_preset then presetIdx = i end
  end

  local rows = {
    ui.row({ gap = 8, align = "center" }, {
      ui.glyph({ name = "headphones" }),
      ui.label({ text = status.name, flexGrow = 1 }),
      ui.label({ text = tostring(status.battery) .. "%" .. (status.charging and (" " .. noctalia.tr("ui.charging")) or "") }),
    }),
    ui.progress({ progress = status.battery / 100 }),
    ui.separator({}),
    ui.label({ text = noctalia.tr("ui.noise_cancelling") }),
    ui.row({ gap = 6 }, {
      modeButton(noctalia.tr("ui.noise_cancelling"), "nc", ""),
      modeButton(noctalia.tr("ui.ambient"), "ambient", " " .. level),
      modeButton(noctalia.tr("ui.off"), "off", ""),
    }),
    ui.row({ gap = 8, align = "center" }, {
      ui.label({ text = noctalia.tr("ui.ambient_level") .. " " .. level }),
      ui.button({ text = "-", enabled = not busy and level > 0, onClick = function ()
        send("mode ambient " .. math.max(0, level - 2)) end }),
      ui.button({ text = "+", enabled = not busy and level < 20, onClick = function ()
        send("mode ambient " .. math.min(20, level + 2)) end }),
    }),
    ui.separator({}),
    ui.label({ text = noctalia.tr("ui.equalizer") }),
    ui.select({
      options = presetLabels,
      selectedIndex = presetIdx - 1,
      enabled = not busy,
      onChange = function (index, text) send("eq preset " .. string.lower(text)) end,
    }),
    ui.row({ gap = 6 }, {
      ui.button({ text = noctalia.tr("ui.ult_off"), variant = status.ult == 0 and "primary" or "secondary", flexGrow = 1, enabled = not busy, onClick = function () send("eq ult 0") end }),
      ui.button({ text = noctalia.tr("ui.ult_1"), variant = status.ult == 1 and "primary" or "secondary", flexGrow = 1, enabled = not busy, onClick = function () send("eq ult 1") end }),
      ui.button({ text = noctalia.tr("ui.ult_2"), variant = status.ult == 2 and "primary" or "secondary", flexGrow = 1, enabled = not busy, onClick = function () send("eq ult 2") end }),
    }),
  }

  for i = 1, 6 do
    local g = status.eq_bands[i]
    table.insert(rows, ui.row({ gap = 8, align = "center" }, {
      ui.label({ text = BANDS[i], flexGrow = 1 }),
      ui.button({ text = "-", enabled = not busy and g > -10, onClick = function () bandStep(i, -1) end }),
      ui.label({ text = string.format("%+d", g), width = 32 }),
      ui.button({ text = "+", enabled = not busy and g < 10, onClick = function () bandStep(i, 1) end }),
    }))
  end

  table.insert(rows, ui.separator({}))
  table.insert(rows, ui.row({ gap = 8, align = "center" }, {
    ui.label({ text = busy and noctalia.tr("ui.busy") or err, flexGrow = 1 }),
    ui.button({ text = noctalia.tr("ui.refresh"), enabled = not busy, onClick = function () send("status") end }),
  }))

  panel.render(ui.column({ padding = 16, gap = 8 }, rows))
end

noctalia.state.watch("ult.status", function (value)
  if value ~= nil and value ~= "" then status = noctalia.json.decode(value) end
  render()
end)
noctalia.state.watch("ult.busy", function (value) busy = value == "1"; render() end)
noctalia.state.watch("ult.error", function (value) err = value or ""; render() end)

function onOpen() render() end
render()
