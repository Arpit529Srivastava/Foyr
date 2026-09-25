"""Convert a static Design-canvas board (.dc.html) into a React component (TSX).

Usage: python3 dc2jsx.py <in.dc.html> <ComponentName> <out.tsx>
Only for boards without {{holes}}, <sc-if>, <sc-for> or <dc-import>.
"""
import re
import sys
from html.parser import HTMLParser

VOID = {"area", "br", "col", "embed", "hr", "img", "input", "link", "meta", "source", "track", "wbr"}
ATTR_MAP = {
    "class": "className", "for": "htmlFor", "tabindex": "tabIndex", "readonly": "readOnly",
    "maxlength": "maxLength", "colspan": "colSpan", "rowspan": "rowSpan", "autocomplete": "autoComplete",
    "viewbox": "viewBox", "preserveaspectratio": "preserveAspectRatio",
}
BOOL_ATTRS = {"disabled", "checked", "readonly", "required", "hidden", "selected", "multiple"}


def camel(prop: str) -> str:
    if prop.startswith("--"):
        return repr(prop)
    if prop.startswith("-webkit-"):
        prop = "Webkit-" + prop[len("-webkit-"):]
    parts = prop.split("-")
    return parts[0] + "".join(p[:1].upper() + p[1:] for p in parts[1:])


def style_obj(css: str) -> str:
    items = []
    for decl in re.split(r";(?![^()]*\))", css):
        if ":" not in decl:
            continue
        k, v = decl.split(":", 1)
        k, v = k.strip(), v.strip()
        if not k:
            continue
        key = camel(k)
        if re.fullmatch(r"-?\d+(\.\d+)?", v) and k in {"opacity", "z-index", "flex-grow", "flex-shrink", "font-weight", "line-height", "order"}:
            items.append(f"{key}: {v}")
        else:
            items.append(f"{key}: {v!r}".replace("\\'", "'"))
    return "{{ " + ", ".join(items) + " }}"


def attr_name(tag: str, name: str) -> str:
    lname = name.lower()
    if lname in ATTR_MAP:
        return ATTR_MAP[lname]
    if lname.startswith(("aria-", "data-")):
        return lname
    if "-" in name:  # svg: stroke-width -> strokeWidth
        return camel(name)
    if name == "viewBox":
        return name
    return name


class Conv(HTMLParser):
    def __init__(self):
        super().__init__(convert_charrefs=False)
        self.out = []
        self.in_body = False
        self.depth = 0
        self.skip = 0  # inside helmet/script

    def handle_starttag(self, tag, attrs):
        if tag == "x-dc":
            self.in_body = True
            return
        if not self.in_body:
            return
        if tag in ("helmet", "script", "style"):
            self.skip += 1
            return
        if self.skip:
            return
        parts = []
        for n, v in attrs:
            if n == "style":
                parts.append(f"style={style_obj(v or '')}")
                continue
            if n.lower().startswith("on"):
                continue
            an = attr_name(tag, n)
            if tag in ("input", "textarea", "select") and an in ("value", "checked"):
                an = "defaultValue" if an == "value" else "defaultChecked"  # static mockup: uncontrolled
            if v is None:
                parts.append(an if n.lower() in BOOL_ATTRS else f'{an}=""')
            else:
                v = v.replace("&amp;", "&")
                parts.append(f"{an}={jsx_str(v)}")
        a = (" " + " ".join(parts)) if parts else ""
        if tag in VOID:
            self.out.append(f"<{tag}{a} />")
        else:
            self.out.append(f"<{tag}{a}>")

    def handle_startendtag(self, tag, attrs):
        self.handle_starttag(tag, attrs)
        if tag not in VOID and self.in_body and not self.skip:
            self.out.append(f"</{tag}>")

    def handle_endtag(self, tag):
        if tag == "x-dc":
            self.in_body = False
            return
        if not self.in_body:
            return
        if tag in ("helmet", "script", "style"):
            self.skip -= 1
            return
        if self.skip or tag in VOID:
            return
        self.out.append(f"</{tag}>")

    def handle_data(self, data):
        if not self.in_body or self.skip:
            return
        if not data.strip():
            if "\n" in data:
                self.out.append("\n")
            return
        # escape braces and angle brackets for JSX text
        esc = data.replace("{", "{'{'}").replace("}", "{'}'}")
        # preserve significant leading/trailing whitespace
        if data[0].isspace() and data.strip():
            esc = "{' '}" + esc.lstrip()
        if data[-1].isspace() and data.strip():
            esc = esc.rstrip() + "{' '}"
        self.out.append(esc)

    def handle_entityref(self, name):
        if self.in_body and not self.skip:
            self.out.append({"amp": "&amp;", "lt": "&lt;", "gt": "&gt;", "nbsp": "&nbsp;", "quot": '"'}.get(name, f"&{name};"))

    def handle_charref(self, name):
        if self.in_body and not self.skip:
            self.out.append(f"&#{name};")


def jsx_str(v: str) -> str:
    if '"' not in v:
        return f'"{v}"'
    return "{" + repr(v) + "}"


def main():
    src, name, dst = sys.argv[1:4]
    html = open(src).read()
    if re.search(r"\{\{|<sc-|<dc-import", html.split("<script type=\"text/x-dc\"")[0]):
        sys.exit(f"{src}: has dynamic parts; convert by hand")
    c = Conv()
    c.feed(html)
    body = "".join(c.out).strip()
    body = re.sub(r"\n{2,}", "\n", body)
    # next/font hashes family names; point at the CSS variables set in app/layout.tsx.
    body = re.sub(r"Inter, ui-sans-serif[^\"]*sans-serif", "var(--font-sans)", body)
    body = re.sub(r"'Geist Mono', 'JetBrains Mono', ui-monospace[^\"]*monospace", "var(--font-mono)", body)
    tsx = (
        "// Generated from the Foyr design canvas (" + src.rsplit('/', 1)[-1] + ") by scripts/dc2jsx.py.\n"
        "// Static product mockup: edit the design, then regenerate, rather than hand-editing.\n\n"
        f"export function {name}() {{\n  return (\n{body}\n  );\n}}\n"
    )
    open(dst, "w").write(tsx)


if __name__ == "__main__":
    main()
