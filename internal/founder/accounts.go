package founder

import (
	"errors"
	"net/http"

	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

// AccountRoles are offered when linking an email to an account.
var accountRoles = []string{"owner", "admin", "member"}

// AccountView is one GitHub account with the people who may open its dashboard.
type AccountView struct {
	store.Installation
	People []store.AccountMember
}

// accounts lists every GitHub account with who may open its dashboard, and the forms to change that.
func (h *Handler) accounts(w http.ResponseWriter, r *http.Request) {
	h.renderAccounts(w, r, http.StatusOK, "")
}

func (h *Handler) renderAccounts(w http.ResponseWriter, r *http.Request, status int, problem string) {
	ctx := r.Context()
	insts, err := h.Data.ListInstallations(ctx)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	views := make([]AccountView, 0, len(insts))
	for _, in := range insts {
		people, err := h.Data.ListAccountMembers(ctx, in.ID)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		views = append(views, AccountView{Installation: in, People: people})
	}
	data := map[string]any{"Accounts": views, "Roles": accountRoles, "Error": problem}
	if status == http.StatusOK {
		h.page(w, r, "accounts.html", "accounts", data)
		return
	}
	// page() always answers 200; render the same page with the error status.
	h.page(&statusWriter{ResponseWriter: w, status: status}, r, "accounts.html", "accounts", data)
}

func (h *Handler) linkPerson(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		http.NotFound(w, r)
		return
	}
	_, err := h.Data.LinkAccountMember(r.Context(), id, r.PostFormValue("email"), r.PostFormValue("role"), actor)
	switch {
	case errors.Is(err, store.ErrBadEmail), errors.Is(err, store.ErrBadAccountRole):
		h.renderAccounts(w, r, http.StatusBadRequest, capitalize(err.Error())+".")
		return
	case errors.Is(err, store.ErrNotFound):
		http.NotFound(w, r)
		return
	case err != nil:
		h.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/founder/accounts", http.StatusSeeOther)
}

func (h *Handler) unlinkPerson(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	uid, ok2 := pathID(r, "uid")
	if !ok || !ok2 {
		http.NotFound(w, r)
		return
	}
	if err := h.Data.UnlinkAccountMember(r.Context(), id, uid, actor); err != nil && !errors.Is(err, store.ErrNotFound) {
		h.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/founder/accounts", http.StatusSeeOther)
}
