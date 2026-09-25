"use client";

import {
  Children,
  createContext,
  isValidElement,
  useContext,
  useRef,
  useState,
  type ComponentProps,
  type ReactNode,
} from "react";
import { ArrowRight, Check, Copy, Info, Lightbulb, TriangleAlert } from "lucide-react";

const InTabs = createContext(false);

function CopyButton({ getText, label = "Copy code" }: { getText: () => string; label?: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <button
      type="button"
      aria-label={copied ? "Copied" : label}
      onClick={async () => {
        try {
          await navigator.clipboard.writeText(getText());
          setCopied(true);
          setTimeout(() => setCopied(false), 1500);
        } catch {
          /* clipboard blocked */
        }
      }}
      className="flex size-8 cursor-pointer items-center justify-center rounded-md text-d-icon transition-colors hover:bg-d-active hover:text-d-strong"
    >
      {copied ? <Check size={14} aria-hidden="true" /> : <Copy size={14} aria-hidden="true" />}
    </button>
  );
}

/** Wraps rehype-pretty-code's <figure>: title bar (from ```lang title="…") plus copy button. */
export function CodeFigure({ children, ...props }: ComponentProps<"figure">) {
  const ref = useRef<HTMLElement>(null);
  const inTabs = useContext(InTabs);
  const kids = Children.toArray(children);
  const caption = kids.find((c) => isValidElement(c) && c.type === "figcaption");
  const rest = kids.filter((c) => c !== caption);
  const text = () => ref.current?.querySelector("code")?.innerText ?? "";

  if (inTabs) {
    return (
      <figure ref={ref} {...props} className="m-0 overflow-x-auto p-4 font-mono text-sm leading-[22px] [&_pre]:font-mono">
        {rest}
      </figure>
    );
  }
  return (
    <figure ref={ref} {...props} className="mt-5 overflow-hidden rounded-xl border border-d-border bg-d-code">
      <div className="flex h-10 items-center justify-between border-b border-d-border pr-1.5 pl-3.5">
        <span className="text-xs font-medium text-d-subtle">
          {caption && isValidElement<{ children?: ReactNode }>(caption) ? caption.props.children : "Code"}
        </span>
        <CopyButton getText={text} />
      </div>
      <div className="overflow-x-auto px-4 py-3.5 font-mono text-sm leading-[22px] [&_pre]:font-mono">{rest}</div>
    </figure>
  );
}

/** <CodeTabs labels={["Next.js", "Express"]}> with one fenced code block per label. */
export function CodeTabs({ labels, children }: { labels: string[]; children: ReactNode }) {
  const [active, setActive] = useState(0);
  const ref = useRef<HTMLDivElement>(null);
  const panes = Children.toArray(children).filter(isValidElement);
  return (
    <div className="mt-5 overflow-hidden rounded-xl border border-d-border bg-d-code">
      <div className="flex h-11 items-center justify-between border-b border-d-border px-1.5">
        <div role="tablist" aria-label="Framework" className="flex gap-0.5">
          {labels.map((l, i) => (
            <button
              key={l}
              type="button"
              role="tab"
              aria-selected={i === active}
              onClick={() => setActive(i)}
              className={`h-8 cursor-pointer rounded-md px-2.5 text-[13px] font-medium transition-colors ${
                i === active ? "bg-d-active text-d-strong" : "text-d-subtle hover:text-d-fg"
              }`}
            >
              {l}
            </button>
          ))}
        </div>
        <CopyButton getText={() => ref.current?.querySelector("code")?.innerText ?? ""} />
      </div>
      <div ref={ref} role="tabpanel">
        <InTabs.Provider value={true}>{panes[active]}</InTabs.Provider>
      </div>
    </div>
  );
}

const CALLOUT = {
  note: { Icon: Info, label: "Note", tone: "" },
  tip: { Icon: Lightbulb, label: "Tip", tone: "" },
  warning: { Icon: TriangleAlert, label: "Warning", tone: "text-d-warn" },
} as const;

export function Callout({ type = "note", children }: { type?: keyof typeof CALLOUT; children: ReactNode }) {
  const { Icon, label, tone } = CALLOUT[type];
  return (
    <div role="note" className="mt-5 flex gap-3 rounded-lg border border-d-border bg-d-callout px-4 py-3.5">
      <Icon size={18} className={`mt-0.5 shrink-0 ${tone || "text-d-icon"}`} aria-hidden="true" />
      <div className="text-sm leading-[22px] text-d-fg [&_p]:m-0 [&_p]:inline [&_p]:text-d-fg">
        <strong className={`mr-1 font-semibold ${tone}`}>{label}</strong> {children}
      </div>
    </div>
  );
}

function DiagramBox({ title, sub, strong }: { title: string; sub: string; strong?: boolean }) {
  return (
    <div className={`rounded-lg border bg-d-bg p-3.5 text-center ${strong ? "w-44 border-d-border2" : "w-40 border-d-border"}`}>
      <div className="text-sm font-medium text-d-strong">{title}</div>
      <div className="mt-0.5 text-xs leading-4 text-d-subtle">{sub}</div>
    </div>
  );
}

function DiagramArrow() {
  return <ArrowRight size={18} className="shrink-0 rotate-90 text-d-subtle sm:rotate-0" aria-hidden="true" />;
}

/** Teammate → Foyr proxy → Your app. */
export function ProxyDiagram({
  caption = "Every request passes the Foyr proxy before it reaches your app.",
}: {
  caption?: string;
}) {
  return (
    <figure className="mt-6 flex flex-col gap-5 rounded-xl border border-d-border bg-d-card px-6 pt-7 pb-5">
      <div className="flex flex-col items-center justify-center gap-3 sm:flex-row">
        <DiagramBox title="Teammate" sub="opens your URL" />
        <DiagramArrow />
        <DiagramBox title="Foyr proxy" sub="checks login and role" strong />
        <DiagramArrow />
        <DiagramBox title="Your app" sub="reads the user" />
      </div>
      <figcaption className="text-center text-[13px] leading-5 text-d-subtle">{caption}</figcaption>
    </figure>
  );
}
