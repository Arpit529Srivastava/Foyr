// Shiki themes matching the Foyr design: monochrome, with amber for function names.
// Plain JSON so they can be passed to MDX plugins under Turbopack (options must be serializable).

const theme = (name, type, c) => ({
  name,
  type,
  colors: { "editor.foreground": c.fg, "editor.background": "#00000000" },
  tokenColors: [
    { settings: { foreground: c.fg } },
    { scope: ["comment", "punctuation.definition.comment"], settings: { foreground: c.kw, fontStyle: "italic" } },
    {
      scope: [
        "keyword",
        "storage",
        "storage.type",
        "storage.modifier",
        "keyword.control",
        "keyword.operator.new",
        "keyword.operator.expression",
        "variable.language",
        "meta.decorator",
        "entity.name.function.decorator",
        "punctuation.definition.decorator",
      ],
      settings: { foreground: c.kw },
    },
    { scope: ["string", "string.quoted", "punctuation.definition.string"], settings: { foreground: c.str } },
    {
      scope: ["entity.name.function", "support.function", "meta.function-call entity.name.function"],
      settings: { foreground: c.acc },
    },
    { scope: ["constant.numeric", "constant.language"], settings: { foreground: c.fg } },
    { scope: ["support.type.property-name.json", "meta.object-literal.key"], settings: { foreground: c.fg } },
  ],
});

export const foyrDark = theme("foyr-dark", "dark", { fg: "#eeeeee", kw: "#8a8a8a", str: "#b5b5b5", acc: "#f5a524" });
export const foyrLight = theme("foyr-light", "light", { fg: "#202020", kw: "#6b6b6b", str: "#525252", acc: "#a86200" });
