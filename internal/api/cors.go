package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

type CORSConfig struct {
	AllowOrigin  string
	AllowMethods []string
	AllowHeaders []string
	MaxAge       int
}

func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowOrigin:  "*",
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowHeaders: []string{"Content-Type", "Authorization", "X-Requested-With", "Accept", "Origin"},
		MaxAge:       86400,
	}
}

type corsHeaders struct {
	allowOrigin  string
	allowMethods string
	allowHeaders string
	maxAge       string
}

func newCORSHeaders(config CORSConfig) corsHeaders {
	return corsHeaders{
		allowOrigin:  config.AllowOrigin,
		allowMethods: strings.Join(config.AllowMethods, ", "),
		allowHeaders: strings.Join(config.AllowHeaders, ", "),
		maxAge:       strconv.Itoa(config.MaxAge),
	}
}

func NewCORSMiddleware(config CORSConfig) func(huma.Context, func(huma.Context)) {
	h := newCORSHeaders(config)
	return func(ctx huma.Context, next func(huma.Context)) {
		ctx.SetHeader("Access-Control-Allow-Origin", h.allowOrigin)
		ctx.SetHeader("Access-Control-Allow-Methods", h.allowMethods)
		ctx.SetHeader("Access-Control-Allow-Headers", h.allowHeaders)
		ctx.SetHeader("Access-Control-Max-Age", h.maxAge)

		if ctx.Method() == http.MethodOptions {
			ctx.SetStatus(http.StatusNoContent)
			return
		}

		next(ctx)
	}
}

func AddCORSHandler(mux *http.ServeMux, config CORSConfig) {
	h := newCORSHeaders(config)
	mux.HandleFunc("OPTIONS /", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", h.allowOrigin)
		w.Header().Set("Access-Control-Allow-Methods", h.allowMethods)
		w.Header().Set("Access-Control-Allow-Headers", h.allowHeaders)
		w.Header().Set("Access-Control-Max-Age", h.maxAge)
		w.WriteHeader(http.StatusNoContent)
	})
}
