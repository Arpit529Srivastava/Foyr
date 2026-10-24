package mail

import "net/url"

// LoginMessage is the sign-in email. link opens the "Continue" page on the auth host; siteURL is the public
// site (https://foyr.online) that hosts the banner image, empty in local development.
func LoginMessage(to, slug, link, siteURL string) Message {
	body := eyebrow("Sign in") + "\n" +
		heading("Sign in to "+esc(slug)+".") + "\n" +
		paragraph("Press the button to finish signing in. No password needed.") + "\n" +
		button(link, "Sign in to "+slug) + "\n" +
		small("This link works once, for 15 minutes. If you didn&#39;t ask for it, ignore this email.") + "\n" +
		small(`If the button doesn&#39;t work, copy this link into your browser:<br /><span style="font-family:`+fontMono+
			`;font-size:12px;color:`+colMuted+`;word-break:break-all;overflow-wrap:anywhere;">`+esc(link)+`</span>`)
	footer := "You&#39;re receiving this because someone asked to sign in to " + esc(slug) +
		" with this email address. Not you? Ignore this email and nothing happens."
	return Message{
		To:      to,
		Subject: "Sign in to " + slug,
		HTML:    page("Sign in to "+slug, "Your sign-in link for "+slug+". It works once, for 15 minutes.", body, footer, siteURL),
		Text: "Sign in to " + slug + ":\n\n" + link + "\n\n" +
			"This link works once, for 15 minutes. If you didn't ask for it, ignore this email.\n",
	}
}

// InviteMessage tells someone they were invited to an app. appURL is the app's address.
func InviteMessage(to, slug, role, appURL, siteURL string) Message {
	host := appURL
	if u, err := url.Parse(appURL); err == nil && u.Host != "" {
		host = u.Host
	}
	body := eyebrow("Invite") + "\n" +
		heading("You&#39;re invited to "+esc(slug)+".") + "\n" +
		paragraph("Foyr invited you to "+strong(slug)+" as "+strong(role)+".") + "\n" +
		details([][2]string{{"App", slug}, {"Role", role}, {"Address", host}}) + "\n" +
		button(appURL, "Open "+slug) + "\n" +
		small("You&#39;ll be asked for your email and sent a sign-in link. No password.")
	footer := "You&#39;re receiving this because you were invited to " + esc(slug) +
		" on Foyr. Not expecting it? Ignore this email."
	return Message{
		To:      to,
		Subject: "You're invited to " + slug,
		HTML:    page("You're invited to "+slug, "You can now open "+slug+" as "+role+".", body, footer, siteURL),
		Text: "Foyr invited you to " + slug + " as " + role + ".\n\nOpen it: " + appURL +
			"\n\nYou'll be asked for your email and sent a sign-in link. No password.\n",
	}
}
