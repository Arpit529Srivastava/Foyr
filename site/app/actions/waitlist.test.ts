import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const pending: Promise<unknown>[] = [];
vi.mock("next/server", () => ({ after: (fn: () => Promise<unknown>) => pending.push(fn()) }));

const { joinWaitlist } = await import("./waitlist");

type Call = { url: string; method: string; body: Record<string, unknown> };

function mockResend({ exists = false, sendStatus = 200 } = {}) {
  const calls: Call[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init: RequestInit = {}) => {
      const method = init.method ?? "GET";
      calls.push({ url, method, body: init.body ? JSON.parse(String(init.body)) : {} });
      if (url.endsWith("/emails")) return new Response("{}", { status: sendStatus });
      if (method === "GET") return new Response("{}", { status: exists ? 200 : 404 });
      return new Response("{}", { status: 201 });
    }),
  );
  return calls;
}

function form(email: string, useCase = "") {
  const f = new FormData();
  f.set("email", email);
  f.set("useCase", useCase);
  return f;
}

async function submit(email: string, useCase = "") {
  const state = await joinWaitlist({ status: "idle" }, form(email, useCase));
  await Promise.all(pending.splice(0));
  return state;
}

const emails = (calls: Call[]) => calls.filter((c) => c.url.endsWith("/emails"));

beforeEach(() => {
  vi.stubEnv("RESEND_API_KEY", "re_test");
  vi.stubEnv("RESEND_AUDIENCE_ID", "aud-1");
  vi.stubEnv("WAITLIST_NOTIFY_EMAIL", "founder@foyr.test");
  vi.stubEnv("WAITLIST_FROM_EMAIL", "");
  vi.spyOn(console, "error").mockImplementation(() => {});
});

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("joinWaitlist emails", () => {
  it("notifies the founder once for a new sign-up, from the shared sender", async () => {
    const calls = mockResend();
    expect(await submit("sam@acme.com", "saas-mvp")).toEqual({ status: "success", email: "sam@acme.com" });
    const sent = emails(calls);
    expect(sent).toHaveLength(1);
    expect(sent[0].body).toMatchObject({
      to: "founder@foyr.test",
      from: "Foyr Waitlist <onboarding@resend.dev>",
      reply_to: "sam@acme.com",
      subject: "New waitlist sign-up: sam@acme.com",
    });
  });

  it("also welcomes the person once a sending domain is set", async () => {
    vi.stubEnv("WAITLIST_FROM_EMAIL", "Foyr <hello@mail.foyr.online>");
    const calls = mockResend();
    await submit("sam@acme.com");
    const sent = emails(calls);
    expect(sent.map((c) => c.body.to).sort()).toEqual(["founder@foyr.test", "sam@acme.com"]);
    expect(sent.every((c) => c.body.from === "Foyr <hello@mail.foyr.online>")).toBe(true);
  });

  it("sends nothing for someone already on the list", async () => {
    const calls = mockResend({ exists: true });
    expect((await submit("sam@acme.com")).status).toBe("already");
    expect(emails(calls)).toHaveLength(0);
  });

  it("calls nothing for an invalid email", async () => {
    const calls = mockResend();
    expect((await submit("not-an-email")).status).toBe("invalid");
    expect(calls).toHaveLength(0);
  });

  it("skips the notification when WAITLIST_NOTIFY_EMAIL is not set", async () => {
    vi.stubEnv("WAITLIST_NOTIFY_EMAIL", "");
    const calls = mockResend();
    expect((await submit("sam@acme.com")).status).toBe("success");
    expect(emails(calls)).toHaveLength(0);
  });

  it("still reports success when sending fails", async () => {
    const calls = mockResend({ sendStatus: 500 });
    expect((await submit("sam@acme.com")).status).toBe("success");
    expect(emails(calls)).toHaveLength(1);
    expect(console.error).toHaveBeenCalled();
  });
});
