package dashboard

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/Arpit529Srivastava/Foyr/internal/mail"
	"github.com/Arpit529Srivastava/Foyr/internal/obs"
	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

// roles are offered in the invite form, in this order.
var roles = []string{"member", "viewer", "admin", "owner"}

// accessPage shows an app's access mode, people and open invites; problem, when set, explains a refused change.
func (h *Handler) accessPage(w http.ResponseWriter, r *http.Request, s scope, status int, problem string) {
	ctx := r.Context()
	members, err := h.Data.ListMembers(ctx, s.App.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	invites, err := h.Data.ListInvites(ctx, s.App.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	list, err := h.Data.ListDeployments(ctx, s.App.ID, 0, historyLimit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var live *store.Deployment
	if i := slices.IndexFunc(list, func(d store.Deployment) bool { return d.ID == s.App.LiveDeployment }); i >= 0 {
		live = &list[i]
	}
	h.pageStatus(w, r, status, s, "access.html", "access", map[string]any{
		"Live": live, "Members": members, "Invites": invites, "Roles": roles, "Error": problem,
	})
}

func (h *Handler) accessURL(s scope) string {
	return "/dashboard/" + s.Account.Login + "/" + s.App.Slug + "/access"
}

func (h *Handler) invite(w http.ResponseWriter, r *http.Request) {
	s, ok := h.load(w, r, true)
	if !ok {
		return
	}
	ctx := r.Context()
	inv, err := h.Data.CreateInvite(ctx, s.App.ID, r.PostFormValue("email"), r.PostFormValue("role"), actor)
	switch {
	case errors.Is(err, store.ErrBadRole), errors.Is(err, store.ErrBadEmail):
		h.accessPage(w, r, s, http.StatusBadRequest, capitalize(err.Error())+".")
		return
	case errors.Is(err, store.ErrAlreadyMember):
		h.accessPage(w, r, s, http.StatusConflict, "That person is already a member.")
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
		link := scheme + "://" + s.App.Slug + "." + h.AppsDomain + "/"
		if err := h.Mail.Send(ctx, mail.InviteMessage(inv.Email, s.App.Slug, inv.Role, link, h.SiteURL)); err != nil {
			obs.Logger(ctx).Error("invite email failed", "app", s.App.Slug, "err", err)
			h.accessPage(w, r, s, http.StatusBadGateway, "The invite is saved, but the email could not be sent.")
			return
		}
	}
	http.Redirect(w, r, h.accessURL(s), http.StatusSeeOther)
}

// id reads a positive number from the path; ok is false after it has answered 404.
func (h *Handler) id(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	n, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || n <= 0 {
		h.notFound(w, r)
		return 0, false
	}
	return n, true
}

func (h *Handler) revokeInvite(w http.ResponseWriter, r *http.Request) {
	s, ok := h.load(w, r, true)
	if !ok {
		return
	}
	iid, ok := h.id(w, r, "iid")
	if !ok {
		return
	}
	// The store revokes by invite id alone, so check it belongs to this app first.
	open, err := h.Data.ListInvites(r.Context(), s.App.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if !slices.ContainsFunc(open, func(i store.Invite) bool { return i.ID == iid }) {
		h.notFound(w, r)
		return
	}
	if err := h.Data.RevokeInvite(r.Context(), iid, actor); err != nil && !errors.Is(err, store.ErrNotFound) {
		h.fail(w, r, err)
		return
	}
	http.Redirect(w, r, h.accessURL(s), http.StatusSeeOther)
}

func (h *Handler) removeMember(w http.ResponseWriter, r *http.Request) {
	s, ok := h.load(w, r, true)
	if !ok {
		return
	}
	uid, ok := h.id(w, r, "uid")
	if !ok {
		return
	}
	err := h.Data.RemoveMember(r.Context(), s.App.ID, uid, actor)
	switch {
	case errors.Is(err, store.ErrNotFound):
		h.notFound(w, r)
		return
	case err != nil:
		h.fail(w, r, err)
		return
	}
	http.Redirect(w, r, h.accessURL(s), http.StatusSeeOther)
}

func (h *Handler) setMode(w http.ResponseWriter, r *http.Request) {
	s, ok := h.load(w, r, true)
	if !ok {
		return
	}
	err := h.Data.SetAccessMode(r.Context(), s.App.ID, r.PostFormValue("mode"), actor)
	switch {
	case errors.Is(err, store.ErrBadMode):
		h.accessPage(w, r, s, http.StatusBadRequest, "Access must be private or public.")
		return
	case errors.Is(err, store.ErrNotFound):
		h.notFound(w, r)
		return
	case err != nil:
		h.fail(w, r, err)
		return
	}
	http.Redirect(w, r, h.accessURL(s), http.StatusSeeOther)
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
