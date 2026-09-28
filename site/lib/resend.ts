import "server-only";

export type OutgoingEmail = { from: string; to: string; subject: string; html: string; text: string; replyTo?: string };

/** Sends one email through Resend; returns false (and logs) on failure instead of throwing. */
export async function sendEmail(apiKey: string, email: OutgoingEmail): Promise<boolean> {
  try {
    const res = await fetch("https://api.resend.com/emails", {
      method: "POST",
      headers: { Authorization: `Bearer ${apiKey}`, "Content-Type": "application/json" },
      body: JSON.stringify({
        from: email.from,
        to: email.to,
        subject: email.subject,
        html: email.html,
        text: email.text,
        ...(email.replyTo ? { reply_to: email.replyTo } : {}),
      }),
      cache: "no-store",
    });
    if (!res.ok) console.error("email: resend error", res.status, await res.text());
    return res.ok;
  } catch (err) {
    console.error("email: request failed", err);
    return false;
  }
}
