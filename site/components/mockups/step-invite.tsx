// Generated from the Foyr design canvas (StepInvite.dc.html) by scripts/dc2jsx.py.
// Static product mockup: edit the design, then regenerate, rather than hand-editing.

export function StepInviteMockup() {
  return (
    <div
      style={{
        width: "900px",
        height: "520px",
        boxSizing: "border-box",
        display: "flex",
        flexDirection: "column",
        overflow: "hidden",
        background: "#000000",
        borderRadius: "16px",
        boxShadow: "0 0 0 1px #ffffff29",
        fontFamily: "var(--font-sans)",
        color: "#eeeeee",
      }}
    >
      <div
        style={{
          height: "44px",
          flexShrink: 0,
          display: "flex",
          alignItems: "center",
          gap: "12px",
          padding: "0 16px",
          background: "#111111",
          borderBottom: "1px solid #2a2a2a",
        }}
      >
        <div style={{ display: "flex", gap: "8px" }}>
          <div
            style={{
              width: "10px",
              height: "10px",
              borderRadius: "9999px",
              background: "#3a3a3a",
            }}
          ></div>
          <div
            style={{
              width: "10px",
              height: "10px",
              borderRadius: "9999px",
              background: "#3a3a3a",
            }}
          ></div>
          <div
            style={{
              width: "10px",
              height: "10px",
              borderRadius: "9999px",
              background: "#3a3a3a",
            }}
          ></div>
        </div>
        <div style={{ flexGrow: 1, display: "flex", justifyContent: "center" }}>
          <div
            style={{
              height: "26px",
              width: "340px",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              gap: "6px",
              background: "#191919",
              border: "1px solid #2a2a2a",
              borderRadius: "6px",
              fontSize: "12px",
              color: "#b5b5b5",
            }}
          >
            <svg
              width="12"
              height="12"
              viewBox="0 0 24 24"
              fill="none"
              stroke="#8a8a8a"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
              aria-hidden="true"
            >
              <rect width="18" height="11" x="3" y="11" rx="2" ry="2"></rect>
              <path d="M7 11V7a5 5 0 0 1 10 0v4"></path>
            </svg>
            <span>app.foyr.online/acme-crm/access</span>
          </div>
        </div>
        <svg
          width="14"
          height="14"
          viewBox="0 0 24 24"
          fill="none"
          stroke="#8a8a8a"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
        >
          <path d="M21 12a9 9 0 1 1-9-9c2.52 0 4.93 1 6.74 2.74L21 8"></path>
          <path d="M21 3v5h-5"></path>
        </svg>
      </div>
      <div
        style={{
          flexGrow: 1,
          boxSizing: "border-box",
          padding: "24px 28px",
          display: "flex",
          gap: "20px",
          alignItems: "flex-start",
        }}
      >
        <div
          style={{
            width: "372px",
            flexShrink: 0,
            display: "flex",
            flexDirection: "column",
            gap: "14px",
          }}
        >
          <div style={{ display: "flex", flexDirection: "column", gap: "4px" }}>
            <div
              style={{
                fontSize: "20px",
                lineHeight: "28px",
                fontWeight: 510,
                letterSpacing: "-0.012em",
                color: "#ffffff",
              }}
            >
              Access
            </div>
            <div style={{ fontSize: "13px", lineHeight: "18px", color: "#a3a3a3" }}>acme-crm.foyrapps.online</div>
          </div>
          <div
            style={{
              display: "flex",
              alignItems: "center",
              gap: "12px",
              padding: "12px 14px",
              background: "#191919",
              border: "1px solid #2a2a2a",
              borderRadius: "12px",
            }}
          >
            <svg
              width="16"
              height="16"
              viewBox="0 0 24 24"
              fill="none"
              stroke="#b5b5b5"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
              aria-hidden="true"
            >
              <rect width="18" height="11" x="3" y="11" rx="2" ry="2"></rect>
              <path d="M7 11V7a5 5 0 0 1 10 0v4"></path>
            </svg>
            <div style={{ flexGrow: 1 }}>
              <div style={{ fontSize: "13px", fontWeight: 500, color: "#eeeeee" }}>Private app</div>
              <div
                style={{
                  fontSize: "12px",
                  lineHeight: "16px",
                  color: "#8a8a8a",
                }}
              >
                Only people below can sign in
              </div>
            </div>
            <button
              type="button"
              role="switch"
              aria-checked="true"
              aria-label="Private app"
              style={{
                width: "34px",
                height: "20px",
                padding: "2px",
                border: "0",
                borderRadius: "9999px",
                background: "#eeeeee",
                display: "flex",
                justifyContent: "flex-end",
              }}
            >
              <div
                style={{
                  width: "16px",
                  height: "16px",
                  borderRadius: "9999px",
                  background: "#000000",
                }}
              ></div>
            </button>
          </div>
          <div style={{ display: "flex", flexDirection: "column", gap: "8px" }}>
            <label htmlFor="step-invite" style={{ fontSize: "13px", fontWeight: 500, color: "#eeeeee" }}>
              Add people
            </label>
            <div style={{ display: "flex", gap: "8px" }}>
              <input
                id="step-invite"
                type="email"
                defaultValue="sam@acme.in"
                style={{
                  flexGrow: 1,
                  minWidth: "0",
                  height: "34px",
                  boxSizing: "border-box",
                  padding: "0 10px",
                  borderRadius: "8px",
                  border: "1px solid #484848",
                  background: "#111111",
                  color: "#eeeeee",
                  fontFamily: "inherit",
                  fontSize: "13px",
                }}
              />
              <button
                type="button"
                style={{
                  height: "34px",
                  display: "flex",
                  alignItems: "center",
                  gap: "6px",
                  padding: "0 10px",
                  borderRadius: "8px",
                  border: "1px solid #2a2a2a",
                  background: "#111111",
                  color: "#eeeeee",
                  fontFamily: "inherit",
                  fontSize: "13px",
                  fontWeight: 500,
                }}
              >
                Member
                <svg
                  width="12"
                  height="12"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="#8a8a8a"
                  strokeWidth="2"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  aria-hidden="true"
                >
                  <path d="m6 9 6 6 6-6"></path>
                </svg>
              </button>
              <button
                type="button"
                style={{
                  height: "34px",
                  padding: "0 12px",
                  borderRadius: "8px",
                  border: "0",
                  background: "#eeeeee",
                  color: "#000000",
                  fontFamily: "inherit",
                  fontSize: "13px",
                  fontWeight: 500,
                }}
              >
                Invite
              </button>
            </div>
          </div>
          <div
            style={{
              background: "#191919",
              border: "1px solid #2a2a2a",
              borderRadius: "12px",
              overflow: "hidden",
            }}
          >
            <div
              style={{
                display: "flex",
                alignItems: "center",
                gap: "10px",
                height: "46px",
                padding: "0 14px",
              }}
            >
              <div
                style={{
                  width: "24px",
                  height: "24px",
                  borderRadius: "9999px",
                  background: "#313131",
                  fontSize: "10px",
                  fontWeight: 600,
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                }}
              >
                PN
              </div>
              <div style={{ flexGrow: 1, fontSize: "13px", color: "#eeeeee" }}>Priya Nair</div>
              <div style={{ fontSize: "12px", color: "#b5b5b5" }}>Owner</div>
            </div>
            <div
              style={{
                display: "flex",
                alignItems: "center",
                gap: "10px",
                height: "46px",
                padding: "0 14px",
                borderTop: "1px solid #2a2a2a",
              }}
            >
              <div
                style={{
                  width: "24px",
                  height: "24px",
                  borderRadius: "9999px",
                  background: "#313131",
                  fontSize: "10px",
                  fontWeight: 600,
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                }}
              >
                RM
              </div>
              <div style={{ flexGrow: 1, fontSize: "13px", color: "#eeeeee" }}>Rahul Mehta</div>
              <div style={{ fontSize: "12px", color: "#b5b5b5" }}>Admin</div>
            </div>
            <div
              style={{
                display: "flex",
                alignItems: "center",
                gap: "10px",
                height: "46px",
                padding: "0 14px",
                borderTop: "1px solid #2a2a2a",
              }}
            >
              <div
                style={{
                  width: "24px",
                  height: "24px",
                  borderRadius: "9999px",
                  background: "#313131",
                  fontSize: "10px",
                  fontWeight: 600,
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                }}
              >
                SO
              </div>
              <div style={{ flexGrow: 1, fontSize: "13px", color: "#eeeeee" }}>Sara Okafor</div>
              <div style={{ fontSize: "12px", color: "#b5b5b5" }}>Member</div>
            </div>
            <div
              style={{
                display: "flex",
                alignItems: "center",
                gap: "10px",
                height: "46px",
                padding: "0 14px",
                borderTop: "1px solid #2a2a2a",
                background: "#222222",
              }}
            >
              <div
                style={{
                  width: "24px",
                  height: "24px",
                  borderRadius: "9999px",
                  background: "#3a3a3a",
                  fontSize: "10px",
                  fontWeight: 600,
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                }}
              >
                SK
              </div>
              <div style={{ flexGrow: 1, fontSize: "13px", color: "#ffffff" }}>sam@acme.in</div>
              <div
                style={{
                  display: "inline-flex",
                  alignItems: "center",
                  height: "20px",
                  padding: "0 8px",
                  borderRadius: "4px",
                  background: "#fbbf241f",
                  color: "#fbbf24",
                  fontSize: "12px",
                  fontWeight: 500,
                }}
              >
                Invited
              </div>
            </div>
          </div>
        </div>
        <div
          style={{
            width: "344px",
            flexShrink: 0,
            boxSizing: "border-box",
            background: "#191919",
            border: "1px solid #2a2a2a",
            borderRadius: "12px",
            overflow: "hidden",
          }}
        >
          <div
            style={{
              display: "flex",
              alignItems: "center",
              gap: "8px",
              height: "40px",
              padding: "0 14px",
              borderBottom: "1px solid #2a2a2a",
              fontSize: "12px",
              fontWeight: 500,
              color: "#8a8a8a",
            }}
          >
            <svg
              width="14"
              height="14"
              viewBox="0 0 24 24"
              fill="none"
              stroke="#8a8a8a"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
              aria-hidden="true"
            >
              <rect width="20" height="16" x="2" y="4" rx="2"></rect>
              <path d="m22 7-8.97 5.7a1.94 1.94 0 0 1-2.06 0L2 7"></path>
            </svg>
            Email preview
          </div>
          <div
            style={{
              display: "grid",
              gridTemplateColumns: "58px minmax(0, 1fr)",
              rowGap: "6px",
              padding: "12px 14px",
              borderBottom: "1px solid #2a2a2a",
              fontSize: "12px",
              lineHeight: "16px",
            }}
          >
            <div style={{ color: "#8a8a8a" }}>From</div>
            <div style={{ color: "#eeeeee" }}>Foyr &lt;login@foyr.online&gt;</div>
            <div style={{ color: "#8a8a8a" }}>To</div>
            <div style={{ color: "#eeeeee" }}>sam@acme.in</div>
            <div style={{ color: "#8a8a8a" }}>Subject</div>
            <div style={{ color: "#eeeeee" }}>Priya invited you to acme-crm</div>
          </div>
          <div
            style={{
              padding: "22px 20px 24px",
              display: "flex",
              flexDirection: "column",
              gap: "14px",
              background: "#111111",
            }}
          >
            <div
              role="img"
              aria-label="Foyr"
              style={{
                display: "flex",
                alignItems: "center",
                gap: "7px",
                color: "#ffffff",
              }}
            >
              <svg width="16" height="16" viewBox="0 0 16 16" fill="#ffffff" shapeRendering="crispEdges" aria-hidden="true">
                <rect x="2" y="1" width="12" height="2"></rect>
                <rect x="2" y="3" width="2" height="12"></rect>
                <rect x="12" y="3" width="2" height="12"></rect>
                <rect x="5" y="4" width="4" height="11"></rect>
              </svg>
              <div
                style={{
                  fontSize: "18px",
                  lineHeight: "20px",
                  fontWeight: 560,
                  letterSpacing: "-0.04em",
                  color: "#ffffff",
                }}
              >
                foyr
              </div>
            </div>
            <div
              style={{
                fontSize: "15px",
                lineHeight: "22px",
                fontWeight: 510,
                color: "#ffffff",
              }}
            >
              Priya Nair invited you to acme-crm as a Member.
            </div>
            <div style={{ fontSize: "13px", lineHeight: "20px", color: "#a3a3a3" }}>
              Open the link to sign in. No password, no account to set up.
            </div>
            <a
              href="#"
              style={{
                alignSelf: "flex-start",
                height: "34px",
                display: "flex",
                alignItems: "center",
                gap: "6px",
                padding: "0 14px",
                borderRadius: "8px",
                background: "#eeeeee",
                color: "#000000",
                fontSize: "13px",
                fontWeight: 500,
                textDecoration: "none",
              }}
            >
              Open acme-crm
              <svg
                width="14"
                height="14"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
                strokeLinejoin="round"
                aria-hidden="true"
              >
                <path d="M5 12h14"></path>
                <path d="m12 5 7 7-7 7"></path>
              </svg>
            </a>
            <div style={{ fontSize: "12px", lineHeight: "16px", color: "#8a8a8a" }}>
              This link signs you in and expires in 24 hours.
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
