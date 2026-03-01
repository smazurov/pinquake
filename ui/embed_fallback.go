//go:build !ui_embed

package ui

import (
	"net/http"
)

func Handler() (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/docs", http.StatusFound)
	}), nil
}
