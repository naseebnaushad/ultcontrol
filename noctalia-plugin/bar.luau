--!nonstrict
-- ULT Control bar widget: headphone glyph + battery %, click opens the panel.

local status = nil
local err = ""

local function render()
  local text = ""
  local glyph = "headphones"
  if status then
    text = tostring(status.battery) .. "%"
  elseif err ~= "" then
    glyph = "headphones-off"
  end
  barWidget.render(ui.row({ align = "center", gap = 4 }, {
    ui.glyph({ name = glyph }),
    ui.label({ text = text }),
  }))
  if status then
    barWidget.setTooltip(status.name .. " - " .. tostring(status.battery) .. "%")
  elseif err ~= "" then
    barWidget.setTooltip(noctalia.tr("ui.offline"))
  end
end

noctalia.state.watch("ult.status", function (value)
  if value ~= nil and value ~= "" then
    status = noctalia.json.decode(value)
  end
  render()
end)
noctalia.state.watch("ult.error", function (value)
  err = value or ""
  render()
end)

function onClick()
  noctalia.togglePanel("mavxd/ultcontrol:panel")
end

render()
