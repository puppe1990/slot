package app

import (
	"net/http"

	"github.com/puppe1990/amarra-cais/pkg/amarra/live"
	"github.com/puppe1990/amarra-cais/pkg/cais"
	"github.com/puppe1990/amarra-cais/pkg/cais/middleware"

	"github.com/puppe1990/slot/internal/handlers"
)

func registerRoutes(r *cais.Router, deps Deps, cfg cais.Config) {
	handlerDeps := handlers.Deps{
		Views:    deps.Views,
		Store:    deps.Store,
		Site:     deps.Site,
		Catalog:  deps.Catalog,
		Config:   cfg,
		Calendar: deps.Calendar,
		Call:     deps.Call,
		Notifier: deps.Notifier,
		Signer:   deps.Signer,
		Clock:    deps.Clock,
	}
	home := handlers.NewHomeHandler(handlerDeps)
	dashboard := handlers.NewDashboardHandler(handlerDeps)
	setup := handlers.NewSetupHandler(handlerDeps)
	meetingTypes := handlers.NewMeetingTypeHandler(handlerDeps)
	booking := handlers.NewBookingHandler(handlerDeps)
	manage := handlers.NewManageHandler(handlerDeps)
	auth := handlers.NewAuthHandler(deps.Views, deps.Store, deps.Site, deps.Store.Sessions(), cfg, deps.Catalog)

	loginLimit := middleware.NewRateLimiter(10, cfg)
	resetLimit := middleware.NewRateLimiter(10, cfg)
	bookingLimit := middleware.NewRateLimiter(30, cfg)
	owner := func(handler http.HandlerFunc) http.HandlerFunc {
		return middleware.RequireAuthFunc("/login", handler)
	}

	r.Get("/", home.ServeHTTP)

	r.Get("/login", auth.Login)
	r.Post("/login", loginLimit.Middleware(http.HandlerFunc(auth.LoginPost)).ServeHTTP)
	r.Get("/signup", auth.SignUp)
	r.Post("/signup", loginLimit.Middleware(http.HandlerFunc(auth.SignUpPost)).ServeHTTP)
	r.Get("/forgot-password", auth.ForgotPassword)
	r.Post("/forgot-password", resetLimit.Middleware(http.HandlerFunc(auth.ForgotPasswordPost)).ServeHTTP)
	r.Get("/reset-password", auth.ResetPassword)
	r.Post("/reset-password", resetLimit.Middleware(http.HandlerFunc(auth.ResetPasswordPost)).ServeHTTP)
	r.Post("/logout", auth.LogoutPost)
	r.Post("/locale", handlers.PostLocale(cfg))

	r.Get("/dashboard", owner(dashboard.ServeHTTP))
	r.Get("/dashboard/export.ics", owner(dashboard.Export))
	r.Get("/bookings/{id}/invite.ics", owner(dashboard.Invite))
	r.Get("/bookings/{id}/reschedule", owner(dashboard.ReschedulePage))
	r.Post("/bookings/{id}/reschedule", owner(dashboard.Reschedule))
	r.Post("/bookings/{id}/cancel", owner(dashboard.Cancel))

	notFound := handlers.NewNotFoundHandler(handlerDeps)
	r.NotFound(notFound.ServeHTTP)

	r.Get("/setup", owner(setup.Show))
	r.Post("/setup", owner(setup.Save))
	r.Post("/setup/overrides", owner(setup.AddOverride))
	r.Post("/setup/overrides/{id}/delete", owner(setup.DeleteOverride))

	r.Get("/meeting-types", owner(meetingTypes.Index))
	r.Post("/meeting-types", owner(meetingTypes.Create))
	r.Get("/meeting-types/{id}/edit", owner(meetingTypes.Edit))
	r.Post("/meeting-types/{id}", owner(meetingTypes.Update))
	r.Post("/meeting-types/{id}/toggle", owner(meetingTypes.Toggle))

	r.Get("/book/{slug}", booking.Show)
	r.Post("/book/{slug}/hold", bookingLimit.Middleware(http.HandlerFunc(booking.Hold)).ServeHTTP)
	r.Post("/book/{slug}/release", bookingLimit.Middleware(http.HandlerFunc(booking.Release)).ServeHTTP)
	r.Get("/book/{slug}/confirm", booking.ConfirmPage)
	r.Post("/book/{slug}/confirm", bookingLimit.Middleware(http.HandlerFunc(booking.Confirm)).ServeHTTP)

	r.Get("/manage/{token}", manage.Show)
	r.Get("/manage/{token}/invite.ics", manage.Invite)
	r.Post("/manage/{token}/reschedule", bookingLimit.Middleware(http.HandlerFunc(manage.Reschedule)).ServeHTTP)
	r.Post("/manage/{token}/cancel", bookingLimit.Middleware(http.HandlerFunc(manage.Cancel)).ServeHTTP)
}

func registerLiveViews(hub *live.Hub, deps Deps) {
	_ = hub
	_ = deps
	// cais:live-views
}
