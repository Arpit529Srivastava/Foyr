"use client";

import { createContext, useActionState, useContext, useEffect, useId, useRef, useState, type ReactNode } from "react";
import { AlertCircle, ArrowRight, Check, Loader2, MailCheck, RotateCw, X } from "lucide-react";
import { joinWaitlist, type WaitlistState } from "@/app/actions/waitlist";
import { buttonStyles } from "@/components/ui";

const WaitlistContext = createContext<{ open: () => void }>({ open: () => {} });

export function WaitlistProvider({ children }: { children: ReactNode }) {
  const [isOpen, setOpen] = useState(false);
  const [session, setSession] = useState(0); // remount the form each time the modal opens
  return (
    <WaitlistContext.Provider
      value={{
        open: () => {
          setSession((s) => s + 1);
          setOpen(true);
        },
      }}
    >
      {children}
      <WaitlistModal key={session} open={isOpen} onClose={() => setOpen(false)} />
    </WaitlistContext.Provider>
  );
}

export function WaitlistButton({
  variant = "primary",
  size = "md",
  className = "",
  children = "Join waitlist",
  arrow = true,
}: {
  variant?: "primary" | "secondary";
  size?: "sm" | "md" | "lg";
  className?: string;
  children?: ReactNode;
  arrow?: boolean;
}) {
  const { open } = useContext(WaitlistContext);
  return (
    <button type="button" onClick={open} className={`${buttonStyles[variant](size)} cursor-pointer ${className}`}>
      {children}
      {arrow && <ArrowRight size={size === "sm" ? 14 : 16} aria-hidden="true" />}
    </button>
  );
}

const USE_CASES = [
  { value: "internal-tool", label: "Internal tool" },
  { value: "client-dashboard", label: "Client dashboard" },
  { value: "saas-mvp", label: "SaaS MVP" },
  { value: "other", label: "Other" },
];

function WaitlistModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const ref = useRef<HTMLDialogElement>(null);
  const emailRef = useRef<HTMLInputElement>(null);
  const titleId = useId();
  const emailId = useId();
  const msgId = useId();
  const [state, action, pending] = useActionState<WaitlistState, FormData>(joinWaitlist, { status: "idle" });
  const [useCase, setUseCase] = useState("");
  const [email, setEmail] = useState("");

  useEffect(() => {
    const d = ref.current;
    if (!d) return;
    if (open && !d.open) {
      d.showModal();
      emailRef.current?.focus();
    }
    if (!open && d.open) d.close();
  }, [open]);

  const done = state.status === "success" || state.status === "already";
  const invalid = state.status === "invalid" && !pending;
  const error = state.status === "error" && !pending;

  return (
    <dialog
      ref={ref}
      aria-labelledby={titleId}
      onClose={onClose}
      onClick={(e) => e.target === ref.current && onClose()}
      className="fixed m-0 mt-auto w-full max-w-none bg-transparent p-0 text-fg backdrop:bg-black/70 backdrop:backdrop-blur-[4px] open:animate-fade sm:m-auto sm:w-[480px]"
    >
      <div className="flex flex-col gap-5 rounded-t-2xl border border-b-0 border-line bg-panel px-4 pt-3 pb-8 sm:rounded-2xl sm:border-b sm:p-7">
        <div aria-hidden="true" className="-mb-1 h-1 w-9 self-center rounded-full bg-[#3a3a3a] sm:hidden" />
        <div className="flex items-start justify-between gap-4">
          <div className="flex flex-col gap-3.5">
            {done && (
              <div className="flex size-10 items-center justify-center rounded-full bg-raised">
                {state.status === "success" ? (
                  <Check size={20} className="text-ok" aria-hidden="true" />
                ) : (
                  <MailCheck size={20} className="text-dim" aria-hidden="true" />
                )}
              </div>
            )}
            <h2 id={titleId} className="text-[22px] leading-7 font-[510] tracking-[-0.012em] text-white">
              {state.status === "success"
                ? "You're on the list."
                : state.status === "already"
                  ? "You're already on the list."
                  : "Get early access"}
            </h2>
          </div>
          <button
            type="button"
            aria-label="Close"
            onClick={onClose}
            className="-mt-1.5 -mr-1.5 flex size-11 shrink-0 cursor-pointer items-center justify-center rounded-lg text-dim transition-colors hover:bg-raised hover:text-white sm:size-8"
          >
            <X size={18} aria-hidden="true" />
          </button>
        </div>

        {done ? (
          <>
            <p role="status" className="-mt-1.5 text-[15px] leading-6 text-muted">
              {state.status === "success"
                ? `We'll email ${state.email} as soon as your spot opens.`
                : `We'll email ${state.email} when your spot opens. Nothing else to do.`}
            </p>
            <button
              type="button"
              onClick={onClose}
              className={`${buttonStyles.secondary("lg")} h-12 cursor-pointer bg-card sm:h-11`}
            >
              Close
            </button>
          </>
        ) : (
          <form action={action} className="flex flex-col gap-5" noValidate>
            <p className="-mt-2 text-sm leading-[22px] text-muted">We&apos;re onboarding developers in small batches.</p>

            <div className={`flex flex-col gap-2 transition-opacity ${pending ? "opacity-50" : ""}`}>
              <label htmlFor={emailId} className="text-[13px] leading-5 font-medium text-fg">
                Email
              </label>
              <input
                ref={emailRef}
                id={emailId}
                name="email"
                type="email"
                autoComplete="email"
                inputMode="email"
                required
                placeholder="you@company.com"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                readOnly={pending}
                aria-invalid={invalid}
                aria-describedby={invalid ? msgId : undefined}
                className={`h-12 rounded-lg border bg-black px-3 text-[15px] text-fg outline-none placeholder:text-subtle focus:border-line-strong focus:shadow-[0_0_0_2px_#111,0_0_0_4px_#f5a524] sm:h-11 ${
                  invalid ? "border-danger" : "border-line"
                }`}
              />
              {invalid && (
                <div id={msgId} className="flex items-center gap-1.5 text-[13px] leading-5 font-medium text-danger">
                  <AlertCircle size={14} aria-hidden="true" />
                  Enter a full email address, like you@company.com.
                </div>
              )}
            </div>

            <fieldset className={`flex flex-col gap-2.5 transition-opacity ${pending ? "opacity-50" : ""}`}>
              <legend className="mb-2.5 text-[13px] leading-5 font-medium text-fg">
                What will you deploy? <span className="font-normal text-subtle">Optional</span>
              </legend>
              <input type="hidden" name="useCase" value={useCase} />
              <div className="flex flex-wrap gap-2">
                {USE_CASES.map((c) => {
                  const on = useCase === c.value;
                  return (
                    <button
                      key={c.value}
                      type="button"
                      aria-pressed={on}
                      disabled={pending}
                      onClick={() => setUseCase(on ? "" : c.value)}
                      className={`flex h-11 cursor-pointer items-center gap-1.5 rounded-full border px-3 text-[13px] font-medium transition-colors sm:h-[34px] ${
                        on
                          ? "border-line-strong bg-raised text-white"
                          : "border-line text-dim hover:border-[#ffffff21] hover:text-fg"
                      }`}
                    >
                      {on && <Check size={14} aria-hidden="true" />}
                      {c.label}
                    </button>
                  );
                })}
              </div>
            </fieldset>

            {error && (
              <div
                role="alert"
                className="flex items-start gap-2 rounded-lg bg-danger/12 px-3 py-2.5 text-[13px] leading-5 font-medium text-danger"
              >
                <AlertCircle size={16} className="mt-0.5 shrink-0" aria-hidden="true" />
                We couldn&apos;t add you just now. Your details are still here, so try again.
              </div>
            )}

            <div className="flex flex-col gap-3">
              <button
                type="submit"
                aria-busy={pending}
                disabled={pending}
                className={`${buttonStyles.primary("lg")} h-12 cursor-pointer disabled:cursor-wait sm:h-11`}
              >
                {pending ? (
                  <>
                    <Loader2 size={16} className="animate-[foyr-spin_800ms_linear_infinite]" aria-hidden="true" />
                    Joining…
                  </>
                ) : error ? (
                  <>
                    <RotateCw size={16} aria-hidden="true" />
                    Try again
                  </>
                ) : (
                  <>
                    Join waitlist
                    <ArrowRight size={16} aria-hidden="true" />
                  </>
                )}
              </button>
              <p className="text-center text-xs leading-4 text-subtle">One email when your spot opens. No spam.</p>
            </div>
          </form>
        )}
      </div>
    </dialog>
  );
}

/** Compact inline form used in the pricing page and final CTA. */
export function WaitlistPill({ className = "" }: { className?: string }) {
  const id = useId();
  const [state, action, pending] = useActionState<WaitlistState, FormData>(joinWaitlist, { status: "idle" });
  const invalid = state.status === "invalid" && !pending;
  const error = state.status === "error" && !pending;

  if (state.status === "success" || state.status === "already") {
    return (
      <div
        role="status"
        className={`flex min-h-12 items-center gap-2.5 rounded-3xl border border-line bg-panel py-[7px] pr-4 pl-[7px] ${className}`}
      >
        <div className="flex size-8 shrink-0 items-center justify-center rounded-full bg-raised">
          {state.status === "success" ? (
            <Check size={16} className="text-ok" aria-hidden="true" />
          ) : (
            <MailCheck size={16} className="text-dim" aria-hidden="true" />
          )}
        </div>
        <span className={`text-sm leading-5 font-medium ${state.status === "success" ? "text-fg" : "text-dim"}`}>
          {state.status === "success"
            ? "You're on the list. We'll email you when your spot opens."
            : "You're already on the list. We'll be in touch."}
        </span>
      </div>
    );
  }

  return (
    <form action={action} aria-label="Join the waitlist" noValidate className={`flex flex-col gap-2.5 ${className}`}>
      <div
        className={`relative flex h-12 items-center gap-2 rounded-full border bg-panel py-1 pr-1 pl-[18px] focus-within:border-line-strong focus-within:shadow-[0_0_0_2px_#000,0_0_0_4px_#f5a524] ${
          invalid ? "border-danger" : "border-line"
        }`}
      >
        <label htmlFor={`${id}-email`} className="sr-only">
          Email address
        </label>
        <input
          id={`${id}-email`}
          name="email"
          type="email"
          autoComplete="email"
          inputMode="email"
          required
          placeholder="you@company.com"
          defaultValue={state.status === "idle" ? "" : state.email}
          readOnly={pending}
          aria-invalid={invalid}
          aria-describedby={invalid || error ? `${id}-msg` : undefined}
          className={`h-[38px] min-w-0 grow bg-transparent text-[15px] outline-none placeholder:text-subtle ${pending ? "text-subtle" : "text-fg"}`}
        />
        <button
          type="submit"
          aria-busy={pending}
          disabled={pending}
          className="flex h-[38px] min-w-[76px] shrink-0 cursor-pointer items-center justify-center gap-1.5 rounded-full bg-fg px-[18px] text-sm font-medium text-black transition-colors hover:bg-white disabled:cursor-wait"
        >
          {pending ? (
            <>
              <Loader2 size={16} className="animate-[foyr-spin_800ms_linear_infinite]" aria-hidden="true" />
              <span className="sr-only">Joining</span>
            </>
          ) : error ? (
            <>
              <RotateCw size={14} aria-hidden="true" />
              Retry
            </>
          ) : (
            "Join"
          )}
        </button>
      </div>
      {(invalid || error) && (
        <div
          id={`${id}-msg`}
          role="alert"
          className="flex items-center gap-1.5 px-[18px] text-[13px] leading-5 font-medium text-danger"
        >
          <AlertCircle size={14} aria-hidden="true" />
          <span>
            {invalid
              ? "Enter a full email address, like you@company.com."
              : "Something went wrong on our side. Try again in a moment."}
          </span>
        </div>
      )}
    </form>
  );
}
