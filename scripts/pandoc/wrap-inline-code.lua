if not FORMAT:match("latex") then
  return
end

local break_after = {
  ["/"] = true,
  ["\\"] = true,
  ["-"] = true,
  ["_"] = true,
  ["."] = true,
  [":"] = true,
  ["?"] = true,
  ["&"] = true,
  ["="] = true,
  [","] = true,
  [";"] = true,
  ["|"] = true,
  [")"] = true,
  ["]"] = true,
}

local function escape_char(ch)
  local map = {
    ["\\"] = "\\textbackslash{}",
    ["{"]  = "\\{",
    ["}"]  = "\\}",
    ["#"]  = "\\#",
    ["$"]  = "\\$",
    ["%"]  = "\\%",
    ["&"]  = "\\&",
    ["_"]  = "\\_",
    ["^"]  = "\\^{}",
    ["~"]  = "\\~{}",
    ["<"]  = "\\textless{}",
    [">"]  = "\\textgreater{}",
  }

  return map[ch] or ch
end

function Code(el)
  local out = {}

  for _, cp in utf8.codes(el.text) do
    local ch = utf8.char(cp)

    if ch == " " or ch == "\n" or ch == "\t" then
      table.insert(out, "\\allowbreak{}\\ ")
    else
      table.insert(out, escape_char(ch))

      if break_after[ch] then
        table.insert(out, "\\allowbreak{}")
      end
    end
  end

  return pandoc.RawInline(
    "latex",
    "\\texttt{" .. table.concat(out) .. "}"
  )
end