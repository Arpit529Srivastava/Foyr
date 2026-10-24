package founder

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Arpit529Srivastava/Foyr/internal/mail"
	"github.com/Arpit529Srivastava/Foyr/internal/obs"
	"github.com/Arpit529Srivastava/Foyr/internal/store"
	"github.com/Arpit529Srivastava/Foyr/internal/web"
)

// actor is who the audit trail names for changes made on this page.
var actor = store.Actor{Type: "system", Login: "founder"}

// guardPost is guard for form posts: same-origin only, small bodies.
func (h *Handler) guardPost(next http.HandlerFunc) http.HandlerFunc {
	return h.guard(func(w http.ResponseWriter, r *http.Request) {
		if !web.SameOrigin(r) {
			obs.Logger(r.Context()).Warn("founder post refused: cross-site", "origin", r.Header.Get("Origin"), "path", r.URL.Path)
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		next(w, r)
	})
}

func pathID(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	return id, err == nil && id > 0
}

// appPage shows an app's access mode, members and open invites, with the forms to change them.
func (h *Handler) appPage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		http.NotFound(w, r)
		return
	}
	h.renderApp(w, r, id, http.StatusOK, "")
}

func (h *Handler) renderApp(w http.ResponseWriter, r *http.Request, id int64, status int, problem string) {
	ctx := r.Context()
	app, err := h.Data.GetApp(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	members, err := h.Data.ListMembers(ctx, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	invites, err := h.Data.ListInvites(ctx, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	data := map[string]any{"App": app, "Members": members, "Invites": invites, "Error": problem, "Roles": []string{"member", "viewer", "admin", "owner"}}
	if status == http.StatusOK {
		h.page(w, r, "app.html", "deployments", data)
		return
	}
	// page() always answers 200; render the same page with the error status.
	rec := &statusWriter{ResponseWriter: w, status: status}
	h.page(rec, r, "app.html", "deployments", data)
}

// statusWriter lets page() keep its signature while an error page carries its own status.
type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (s *statusWriter) WriteHeader(int) {
	if !s.wrote {
		s.wrote = true
		s.ResponseWriter.WriteHeader(s.status)
	}
}

func (s *statusWriter) Write(b []byte) (int, error) {
	s.WriteHeader(0)
	return s.ResponseWriter.Write(b)
}

func (h *Handler) back(w http.ResponseWriter, r *http.Request, id int64) {
	http.Redirect(w, r, "/founder/apps/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (h *Handler) invite(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()
	app, err := h.Data.GetApp(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	inv, err := h.Data.CreateInvite(ctx, id, r.PostFormValue("email"), r.PostFormValue("role"), actor)
	switch {
	case errors.Is(err, store.ErrBadRole), errors.Is(err, store.ErrBadEmail):
		h.renderApp(w, r, id, http.StatusBadRequest, capitalize(err.Error())+".")
		return
	case errors.Is(err, store.ErrAlreadyMember):
		h.renderApp(w, r, id, http.StatusConflict, "That person is already a member.")
		return
	case err != nil:
		h.fail(w, r, err)
		return
	}
	if h.Mail != nil {
		scheme := h.AppsScheme
		if scheme == "" {
			scheme = "https"
		}
		link := scheme + "://" + app.Slug + "." + h.AppsDomain + "/"
		msg := mail.Message{
			To:      inv.Email,
			Subject: "You're invited to " + app.Slug,
			Text: "Foyr invited you to " + app.Slug + " as " + inv.Role + ".\n\nOpen it: " + link +
				"\n\nYou'll be asked for your email and sent a sign-in link. No password.\n",
		}
		if err := h.Mail.Send(ctx, msg); err != nil {
			obs.Logger(ctx).Error("invite email failed", "app", app.Slug, "err", err)
			h.renderApp(w, r, id, http.StatusBadGateway, "The invite is saved, but the email could not be sent. Check the mail settings.")
			return
		}
	}
	h.back(w, r, id)
}

func (h *Handler) revokeInvite(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	iid, ok2 := pathID(r, "iid")
	if !ok || !ok2 {
		http.NotFound(w, r)
		return
	}
	if err := h.Data.RevokeInvite(r.Context(), iid, actor); err != nil && !errors.Is(err, store.ErrNotFound) {
		h.fail(w, r, err)
		return
	}
	h.back(w, r, id)
}

func (h *Handler) removeMember(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	uid, ok2 := pathID(r, "uid")
	if !ok || !ok2 {
		http.NotFound(w, r)
		return
	}
	if err := h.Data.RemoveMember(r.Context(), id, uid, actor); err != nil && !errors.Is(err, store.ErrNotFound) {
		h.fail(w, r, err)
		return
	}
	h.back(w, r, id)
}

func (h *Handler) setAccess(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		http.NotFound(w, r)
		return
	}
	err := h.Data.SetAccessMode(r.Context(), id, r.PostFormValue("mode"), actor)
	switch {
	case errors.Is(err, store.ErrBadMode):
		h.renderApp(w, r, id, http.StatusBadRequest, "Access must be private or public.")
		return
	case errors.Is(err, store.ErrNotFound):
		http.NotFound(w, r)
		return
	case err != nil:
		h.fail(w, r, err)
		return
	}
	h.back(w, r, id)
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
