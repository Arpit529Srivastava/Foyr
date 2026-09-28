"use server";

import { after } from "next/server";
import { founderNotifyEmail, welcomeEmail } from "@/lib/emails/waitlist";
import { sendEmail } from "@/lib/resend";
import { siteUrl } from "@/lib/site";
import { isUseCase } from "@/lib/waitlist";

export type WaitlistState =
  | { status: "idle" }
  | { status: "invalid"; email: string }
  | { status: "success"; email: string }
  | { status: "already"; email: string }
  | { status: "error"; email: string };

// Resend's shared sender; only delivers to the Resend account owner until a domain is verified.
const DEFAULT_FROM = "Foyr Waitlist <onboarding@resend.dev>";
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

  const base = `https://api.resend.com/audiences/${audience}/contacts`;
  const headers = { Authorization: `Bearer ${apiKey}`, "Content-Type": "application/json" };

  try {
    // Resend upserts on POST (a repeat email returns 201 and would overwrite the stored use case),
    // so look the contact up first to tell "new" from "already on the list".
    const existing = await fetch(`${base}/${encodeURIComponent(email)}`, { headers, cache: "no-store" });
    if (existing.ok) return { status: "already", email };
    if (existing.status !== 404) {
      console.error("waitlist: resend lookup error", existing.status, await existing.text());
      return { status: "error", email };
    }

    const res = await fetch(base, {
      method: "POST",
      headers,
      body: JSON.stringify({
        email,
        unsubscribed: false,
        ...(isUseCase(useCase) ? { first_name: `uc:${useCase}` } : {}),
      }),
      cache: "no-store",
    });
    if (res.ok) {
      after(() => sendWaitlistEmails(apiKey, audience, email, useCase));
      return { status: "success", email };
    }
    console.error("waitlist: resend error", res.status, await res.text());
    return { status: "error", email };
  } catch (err) {
    console.error("waitlist: request failed", err);
    return { status: "error", email };
  }
}

/**
 * After a new sign-up: notify the founder (WAITLIST_NOTIFY_EMAIL) and, once a sending domain is
 * verified (WAITLIST_FROM_EMAIL), welcome the person who joined. Failures are logged only.
 */
async function sendWaitlistEmails(apiKey: string, audience: string, email: string, useCase: string) {
  const from = process.env.WAITLIST_FROM_EMAIL || undefined;
  const notifyTo = process.env.WAITLIST_NOTIFY_EMAIL || undefined;
  const jobs: Promise<boolean>[] = [];
  if (notifyTo) {
    const message = founderNotifyEmail({ email, useCase, at: new Date(), audienceId: audience, siteUrl });
    jobs.push(sendEmail(apiKey, { from: from ?? DEFAULT_FROM, to: notifyTo, replyTo: email, ...message }));
  }
  if (from) {
    jobs.push(sendEmail(apiKey, { from, to: email, ...welcomeEmail({ email, useCase, siteUrl }) }));
  }
  await Promise.all(jobs);
}
