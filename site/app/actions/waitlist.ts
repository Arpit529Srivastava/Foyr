"use server";

export type WaitlistState =
  | { status: "idle" }
  | { status: "invalid"; email: string }
  | { status: "success"; email: string }
  | { status: "already"; email: string }
  | { status: "error"; email: string };

const USE_CASES = new Set(["internal-tool", "client-dashboard", "saas-mvp", "other"]);
const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]{2,}$/;

/**
 * Adds an email to the Resend audience configured by RESEND_API_KEY + RESEND_AUDIENCE_ID.
 * The optional use case is stored as the contact's first name field ("uc:<value>") so it
 * shows up in the Resend dashboard without a separate database.
 */
export async function joinWaitlist(_prev: WaitlistState, form: FormData): Promise<WaitlistState> {
  const email = String(form.get("email") ?? "")
    .trim()
    .toLowerCase();
  const useCase = String(form.get("useCase") ?? "");
  if (!EMAIL.test(email) || email.length > 254) return { status: "invalid", email };

  const apiKey = process.env.RESEND_API_KEY;
  const audience = process.env.RESEND_AUDIENCE_ID;
  if (!apiKey || !audience) {
    console.error("waitlist: RESEND_API_KEY / RESEND_AUDIENCE_ID not set");
    return { status: "error", email };
  }

  try {
    const res = await fetch(`https://api.resend.com/audiences/${audience}/contacts`, {
      method: "POST",
      headers: { Authorization: `Bearer ${apiKey}`, "Content-Type": "application/json" },
      body: JSON.stringify({
        email,
        unsubscribed: false,
        ...(USE_CASES.has(useCase) ? { first_name: `uc:${useCase}` } : {}),
      }),
      cache: "no-store",
    });
    if (res.ok) return { status: "success", email };
    const body = await res.text();
    if (res.status === 409 || /already exists/i.test(body)) return { status: "already", email };
    console.error("waitlist: resend error", res.status, body);
    return { status: "error", email };
  } catch (err) {
    console.error("waitlist: request failed", err);
    return { status: "error", email };
  }
}
