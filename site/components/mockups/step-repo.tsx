// Generated from the Foyr design canvas (StepRepo.dc.html) by scripts/dc2jsx.py.
// Static product mockup: edit the design, then regenerate, rather than hand-editing.

export function StepRepoMockup() {
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
            <span>app.foyr.online/new</span>
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
          padding: "28px 40px",
          display: "flex",
          flexDirection: "column",
          gap: "18px",
          width: "740px",
        }}
      >
        <div style={{ display: "flex", flexDirection: "column", gap: "6px" }}>
          <div
            style={{
              fontSize: "20px",
              lineHeight: "28px",
              fontWeight: 510,
              letterSpacing: "-0.012em",
              color: "#ffffff",
            }}
          >
            Import a repository
          </div>
          <div style={{ fontSize: "14px", lineHeight: "22px", color: "#a3a3a3" }}>
            Foyr only sees the repos you pick. Read-only, per repo.
          </div>
        </div>
        <div style={{ display: "flex", gap: "8px" }}>
          <button
            type="button"
            style={{
              height: "36px",
              display: "flex",
              alignItems: "center",
              gap: "8px",
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
            <div
              style={{
                width: "18px",
                height: "18px",
                borderRadius: "4px",
                background: "#eeeeee",
                color: "#000000",
                fontSize: "10px",
                fontWeight: 600,
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
              }}
            >
              A
            </div>
            acme
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
          <div
            style={{
              flexGrow: 1,
              height: "36px",
              boxSizing: "border-box",
              display: "flex",
              alignItems: "center",
              gap: "8px",
              padding: "0 10px",
              borderRadius: "8px",
              border: "1px solid #2a2a2a",
              background: "#111111",
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
              <circle cx="11" cy="11" r="8"></circle>
              <path d="m21 21-4.3-4.3"></path>
            </svg>
            <input
              type="search"
              aria-label="Search repositories"
              placeholder="Search repositories"
              style={{
                flexGrow: 1,
                border: "0",
                background: "transparent",
                color: "#eeeeee",
                fontFamily: "inherit",
                fontSize: "13px",
                outline: "none",
              }}
            />
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
              gap: "12px",
              height: "58px",
              padding: "0 16px",
              background: "#222222",
            }}
          >
            <div
              style={{
                width: "30px",
                height: "30px",
                flexShrink: 0,
                borderRadius: "6px",
                background: "#2a2a2a",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                fontFamily: "var(--font-mono)",
                fontSize: "11px",
                fontWeight: 500,
                color: "#eeeeee",
              }}
            >
              TS
            </div>
            <div style={{ flexGrow: 1, minWidth: "0" }}>
              <div style={{ fontSize: "14px", fontWeight: 500, color: "#ffffff" }}>acme-crm</div>
              <div
                style={{
                  fontSize: "12px",
                  lineHeight: "16px",
                  color: "#8a8a8a",
                }}
              >
                Next.js · Updated 2m ago
              </div>
            </div>
            <button
              type="button"
              style={{
                height: "32px",
                padding: "0 14px",
                borderRadius: "8px",
                border: "0",
                background: "#eeeeee",
                color: "#000000",
                fontFamily: "inherit",
                fontSize: "13px",
                fontWeight: 500,
              }}
            >
              Connect
            </button>
          </div>
          <div
            style={{
              display: "flex",
              alignItems: "center",
              gap: "12px",
              height: "58px",
              padding: "0 16px",
              borderTop: "1px solid #2a2a2a",
            }}
          >
            <div
              style={{
                width: "30px",
                height: "30px",
                flexShrink: 0,
                borderRadius: "6px",
                background: "#2a2a2a",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                fontFamily: "var(--font-mono)",
                fontSize: "11px",
                fontWeight: 500,
                color: "#eeeeee",
              }}
            >
              TS
            </div>
            <div style={{ flexGrow: 1, minWidth: "0" }}>
              <div style={{ fontSize: "14px", fontWeight: 500, color: "#eeeeee" }}>invoice-portal</div>
              <div
                style={{
                  fontSize: "12px",
                  lineHeight: "16px",
                  color: "#8a8a8a",
                }}
              >
                Next.js · Updated 3h ago
              </div>
            </div>
            <button
              type="button"
              style={{
                height: "32px",
                padding: "0 14px",
                borderRadius: "8px",
                border: "1px solid #2a2a2a",
                background: "#111111",
                color: "#eeeeee",
                fontFamily: "inherit",
                fontSize: "13px",
                fontWeight: 500,
              }}
            >
              Connect
            </button>
          </div>
          <div
            style={{
              display: "flex",
              alignItems: "center",
              gap: "12px",
              height: "58px",
              padding: "0 16px",
              borderTop: "1px solid #2a2a2a",
            }}
          >
            <div
              style={{
                width: "30px",
                height: "30px",
                flexShrink: 0,
                borderRadius: "6px",
                background: "#2a2a2a",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                fontFamily: "var(--font-mono)",
                fontSize: "11px",
                fontWeight: 500,
                color: "#eeeeee",
              }}
            >
              PY
            </div>
            <div style={{ flexGrow: 1, minWidth: "0" }}>
              <div style={{ fontSize: "14px", fontWeight: 500, color: "#eeeeee" }}>ops-dashboard</div>
              <div
                style={{
                  fontSize: "12px",
                  lineHeight: "16px",
                  color: "#8a8a8a",
                }}
              >
                FastAPI · Updated yesterday
              </div>
            </div>
            <button
              type="button"
              style={{
                height: "32px",
                padding: "0 14px",
                borderRadius: "8px",
                border: "1px solid #2a2a2a",
                background: "#111111",
                color: "#eeeeee",
                fontFamily: "inherit",
                fontSize: "13px",
                fontWeight: 500,
              }}
            >
              Connect
            </button>
          </div>
          <div
            style={{
              display: "flex",
              alignItems: "center",
              gap: "12px",
              height: "58px",
              padding: "0 16px",
              borderTop: "1px solid #2a2a2a",
            }}
          >
            <div
              style={{
                width: "30px",
                height: "30px",
                flexShrink: 0,
                borderRadius: "6px",
                background: "#2a2a2a",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                fontFamily: "var(--font-mono)",
                fontSize: "11px",
                fontWeight: 500,
                color: "#eeeeee",
              }}
            >
              JS
            </div>
            <div style={{ flexGrow: 1, minWidth: "0" }}>
              <div style={{ fontSize: "14px", fontWeight: 500, color: "#eeeeee" }}>client-reports</div>
              <div
                style={{
                  fontSize: "12px",
                  lineHeight: "16px",
                  color: "#8a8a8a",
                }}
              >
                Express · Updated 3 days ago
              </div>
            </div>
            <div
              style={{
                height: "32px",
                display: "flex",
                alignItems: "center",
                gap: "6px",
                padding: "0 6px",
                fontSize: "13px",
                fontWeight: 500,
                color: "#4ade80",
              }}
            >
              <svg
                width="14"
                height="14"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="2.5"
                strokeLinecap="round"
                strokeLinejoin="round"
                aria-hidden="true"
              >
                <path d="M20 6 9 17l-5-5"></path>
              </svg>
              Connected
            </div>
          </div>
          <div
            style={{
              display: "flex",
              alignItems: "center",
              gap: "12px",
              height: "58px",
              padding: "0 16px",
              borderTop: "1px solid #2a2a2a",
            }}
          >
            <div
              style={{
                width: "30px",
                height: "30px",
                flexShrink: 0,
                borderRadius: "6px",
                background: "#2a2a2a",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                fontFamily: "var(--font-mono)",
                fontSize: "11px",
                fontWeight: 500,
                color: "#eeeeee",
              }}
            >
              TS
            </div>
            <div style={{ flexGrow: 1, minWidth: "0" }}>
              <div style={{ fontSize: "14px", fontWeight: 500, color: "#eeeeee" }}>partner-onboarding</div>
              <div
                style={{
                  fontSize: "12px",
                  lineHeight: "16px",
                  color: "#8a8a8a",
                }}
              >
                Next.js · Updated last week
              </div>
            </div>
            <button
              type="button"
              style={{
                height: "32px",
                padding: "0 14px",
                borderRadius: "8px",
                border: "1px solid #2a2a2a",
                background: "#111111",
                color: "#eeeeee",
                fontFamily: "inherit",
                fontSize: "13px",
                fontWeight: 500,
              }}
            >
              Connect
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
