package mail

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const (
	testLink = "https://auth.foyr.online/login/verify?app=foyr-hello&return=%2Freports&t=abc_DEF-123"
	testSite = "https://foyr.online"
)

func TestLoginMessage(t *testing.T) {
	m := LoginMessage("ana@example.com", "foyr-hello", testLink, testSite)
	if m.To != "ana@example.com" || m.Subject != "Sign in to foyr-hello" {
		t.Fatalf("to %q subject %q", m.To, m.Subject)
	}

	// The plain text stays minimal: one link, the once-for-15-minutes note.
	if !strings.Contains(m.Text, testLink) || !strings.Contains(m.Text, "once, for 15 minutes") ||
		!strings.Contains(m.Text, "ignore this email") {
		t.Errorf("text = %q", m.Text)
	}

	// The HTML uses the waitlist email's design.
	for _, want := range []string{
		`background:#000000`,                       // black page
		`background:#191919`,                       // the card
		testSite + `/email/banner.jpg`,             // the doorway-light banner
		`>foyr</td>`,                               // the wordmark
		`Sign in</td>`,                             // the mono eyebrow
		`Sign in to foyr-hello`,                    // heading and button
		`background:#eeeeee`,                       // the white pill button
		`once, for 15 minutes`,                     // the safety note
		`If you didn&#39;t ask for it`,             // escaped apostrophe
		`href="` + testSite + `"`,                  // logo links to the site
		`<meta name="color-scheme" content="dark"`, // dark scheme hint
	} {
		if !strings.Contains(m.HTML, want) {
			t.Errorf("HTML lacks %q", want)
		}
	}
}

func TestLoginMessageButtonAndFallbackLinkAreTheSame(t *testing.T) {
	m := LoginMessage("ana@example.com", "foyr-hello", testLink, testSite)
	// href values are HTML-escaped (& becomes &amp;); unescaping must give back the exact link.
	re := regexp.MustCompile(`href="([^"]*login/verify[^"]*)"`)
	hrefs := re.FindAllStringSubmatch(m.HTML, -1)
	if len(hrefs) != 1 {
		t.Fatalf("%d verify links in the HTML, want exactly the button", len(hrefs))
	}
	if got := strings.ReplaceAll(hrefs[0][1], "&amp;", "&"); got != testLink {
		t.Errorf("button links to %q, want %q", got, testLink)
	}
	// The token appears in the button and in the copy-paste fallback, and nowhere else (not the preheader or title).
	if n := strings.Count(m.HTML, "abc_DEF-123"); n != 2 {
		t.Errorf("token appears %d times in the HTML, want 2 (button and fallback)", n)
	}
}

func TestLoginMessageEscapesWhatItInterpolates(t *testing.T) {
	m := LoginMessage(`a"@example.com`, `<script>alert(1)</script>`, `https://auth.foyr.online/x?a=1&b="2"`, testSite)
	if strings.Contains(m.HTML, "<script>") || strings.Contains(m.HTML, `b="2"`) {
		t.Errorf("unescaped input reached the HTML")
	}
	if !strings.Contains(m.HTML, "&lt;script&gt;") {
		t.Errorf("the slug should appear escaped")
	}
}

func TestInviteMessage(t *testing.T) {
	m := InviteMessage("ana@example.com", "foyr-hello", "owner", "https://foyr-hello.apps.foyr.online/", testSite)
	if m.To != "ana@example.com" || m.Subject != "You're invited to foyr-hello" {
		t.Fatalf("to %q subject %q", m.To, m.Subject)
	}
	if !strings.Contains(m.Text, "invited you to foyr-hello") || !strings.Contains(m.Text, "owner") ||
		!strings.Contains(m.Text, "https://foyr-hello.apps.foyr.online/") || !strings.Contains(m.Text, "No password") {
		t.Errorf("text = %q", m.Text)
	}
	for _, want := range []string{
		testSite + `/email/banner.jpg`,
		`Invite</td>`,                       // eyebrow
		`You&#39;re invited to foyr-hello.`, // heading
		`href="https://foyr-hello.apps.foyr.online/"`,
		`Open foyr-hello`, // button
		`owner`,           // role in the details panel
		`No password`,
		`background:#eeeeee`,
	} {
		if !strings.Contains(m.HTML, want) {
			t.Errorf("HTML lacks %q", want)
		}
	}
	if !strings.Contains(m.HTML, "foyr-hello.apps.foyr.online") {
		t.Error("the app address should be shown")
	}
}

func TestMessagesWithoutASiteURLHaveNoBrokenImage(t *testing.T) {
	// Local development has no public site, so no banner image rather than a broken one.
	for name, m := range map[string]Message{
		"login":  LoginMessage("a@b.co", "crm", "http://localhost:8090/login/verify?t=x", ""),
		"invite": InviteMessage("a@b.co", "crm", "member", "http://crm.apps.localhost:8081/", ""),
	} {
		if strings.Contains(m.HTML, "<img") || strings.Contains(m.HTML, "banner.jpg") {
			t.Errorf("%s: HTML references an image without a site URL", name)
		}
		if !strings.Contains(m.HTML, "foyr") {
			t.Errorf("%s: wordmark missing", name)
		}
	}
}

func TestHTMLIsEmailSafe(t *testing.T) {
	// No scripts, no external CSS besides the font link, tables for layout like the waitlist emails.
	for name, m := range map[string]Message{
		"login":  LoginMessage("a@b.co", "crm", testLink, testSite),
		"invite": InviteMessage("a@b.co", "crm", "member", "https://crm.apps.foyr.online/", testSite),
	} {
		low := strings.ToLower(m.HTML)
		for _, bad := range []string{"<script", "javascript:", "<style", "onclick="} {
			if strings.Contains(low, bad) {
				t.Errorf("%s: HTML contains %q", name, bad)
			}
		}
		if !strings.Contains(low, `role="presentation"`) || !strings.HasPrefix(m.HTML, "<!doctype html>") {
			t.Errorf("%s: not a table-based HTML document", name)
		}
	}
}

// With FOYR_EMAIL_PREVIEW_DIR set, writes the emails as HTML files to look at in a browser.
func TestWritePreviews(t *testing.T) {
	dir := os.Getenv("FOYR_EMAIL_PREVIEW_DIR")
	if dir == "" {
		t.Skip("set FOYR_EMAIL_PREVIEW_DIR to write preview files")
	}
	for name, m := range map[string]Message{
		"login":  LoginMessage("ana@example.com", "foyr-hello", testLink, testSite),
		"invite": InviteMessage("ana@example.com", "foyr-hello", "owner", "https://foyr-hello.apps.foyr.online/", testSite),
	} {
		if err := os.WriteFile(filepath.Join(dir, name+".html"), []byte(m.HTML), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDashboardLoginMessage(t *testing.T) {
	link := "https://app.foyr.online/dashboard/login/verify?t=abc_DEF-123"
	m := DashboardLoginMessage("ana@example.com", link, testSite)
	if m.To != "ana@example.com" || m.Subject != "Sign in to Foyr" {
		t.Fatalf("to %q subject %q", m.To, m.Subject)
	}
	if !strings.Contains(m.Text, link) || !strings.Contains(m.Text, "once, for 15 minutes") {
		t.Errorf("text = %q", m.Text)
	}
	// The link appears as the button and again as the fallback text, both escaped the same way.
	if n := strings.Count(m.HTML, link); n != 2 {
		t.Errorf("link appears %d times in the HTML, want 2 (button and fallback)", n)
	}
	for _, want := range []string{"Open the dashboard", "once, for 15 minutes", testSite + "/email/banner.jpg"} {
		if !strings.Contains(m.HTML, want) {
			t.Errorf("HTML is missing %q", want)
		}
	}
	if strings.Contains(DashboardLoginMessage("a@b.test", link, "").HTML, "banner.jpg") {
		t.Error("an empty site URL still has a banner image")
	}
}
