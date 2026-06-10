package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Router interface {
	Use(middlewares ...func(http.Handler) http.Handler)
	Get(pattern string, handler http.HandlerFunc)
	Post(pattern string, handler http.HandlerFunc)
	Group(pattern string, register func(Router))
	Handler() http.Handler
}

type ChiRouter struct {
	router chi.Router
}

func NewRouter() *ChiRouter {
	return &ChiRouter{router: chi.NewRouter()}
}

func DefaultMiddlewares() []func(http.Handler) http.Handler {
	return []func(http.Handler) http.Handler{
		middleware.RequestID,
		middleware.RealIP,
		middleware.Recoverer,
	}
}

func (r *ChiRouter) Use(middlewares ...func(http.Handler) http.Handler) {
	r.router.Use(middlewares...)
}

func (r *ChiRouter) Get(pattern string, handler http.HandlerFunc) {
	r.router.Get(pattern, handler)
}

func (r *ChiRouter) Post(pattern string, handler http.HandlerFunc) {
	r.router.Post(pattern, handler)
}

func (r *ChiRouter) Group(pattern string, register func(Router)) {
	r.router.Route(pattern, func(router chi.Router) {
		register(&ChiRouter{router: router})
	})
}

func (r *ChiRouter) Handler() http.Handler {
	return r.router
}

func URLParam(r *http.Request, key string) string {
	return chi.URLParam(r, key)
}
