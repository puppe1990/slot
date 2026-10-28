package handlers

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/puppe1990/amarra-cais/pkg/amarra/view"
	"github.com/puppe1990/amarra-cais/pkg/cais"
	"github.com/puppe1990/amarra-cais/pkg/cais/i18n"
	"github.com/puppe1990/amarra-cais/pkg/cais/meta"
	"github.com/puppe1990/amarra-cais/pkg/cais/session"

	"github.com/puppe1990/slot/internal/calendar"
	"github.com/puppe1990/slot/internal/call"
	"github.com/puppe1990/slot/internal/guesttoken"
	"github.com/puppe1990/slot/internal/models"
	"github.com/puppe1990/slot/internal/notify"
	"github.com/puppe1990/slot/internal/store"
)

// Deps is the single wiring point for the scheduling handlers. Tests build the
// same struct with a fake clock and the in-memory calendar, so no handler
// reaches for a global.
type Deps struct {
	Views    *view.Renderer
	Store    store.Store
	Site     meta.Site
	Catalog  *i18n.Catalog
	Catalogs map[string]*i18n.Catalog
	Config   cais.Config
	Calendar calendar.Provider
	Call     call.Provider
	Notifier notify.Notifier
	Signer   *guesttoken.Signer
	Clock    func() time.Time
}

type base struct {
	deps Deps
}

func newBase(deps Deps) base {
	if deps.Calendar == nil {
		deps.Calendar = calendar.NewFake()
	}
	if deps.Call == nil {
		deps.Call = call.None{}
	}
	if deps.Notifier == nil {
		deps.Notifier = notify.NewOutbox()
	}
	if deps.Clock == nil {
		deps.Clock = func() time.Time { return time.Now().UTC() }
	}
	return base{deps: deps}
}

func (b base) now() time.Time {
	return b.deps.Clock().UTC()
}

func (b base) page(w http.ResponseWriter, r *http.Request, layout, name string, extra map[string]any, status int) {
	writeView(w, r, b.deps.Views, b.deps.Config, layout, name, amarraData(r, b.deps.Site, extra), status)
}

// absoluteURL is what guests paste into a chat: a relative path is useless
// outside the browser.
func (b base) absoluteURL(r *http.Request, path string) string {
	if path == "" {
		return ""
	}
	root := strings.TrimRight(b.deps.Site.AppURL, "/")
	if root == "" {
		root = "http://" + r.Host
	}
	return root + path
}

func (b base) ownerID(r *http.Request) int64 {
	id, _ := session.UserID(r)
	return id
}

func (b base) serverError(w http.ResponseWriter, r *http.Request, err error) {
	log.Printf("slot: %s %s: %v", r.Method, r.URL.Path, err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}

func (b base) owner(r *http.Request) (ownerUser, bool) {
	userID := b.ownerID(r)
	if userID == 0 {
		return ownerUser{}, false
	}
	user, err := b.deps.Store.GetUser(userID)
	if err != nil {
		log.Printf("slot: load owner %d: %v", userID, err)
		return ownerUser{}, false
	}
	location, err := time.LoadLocation(user.Timezone)
	if err != nil {
		location = time.UTC
	}
	return ownerUser{User: user, Location: location}, true
}

type ownerUser struct {
	User     models.User
	Location *time.Location
}
