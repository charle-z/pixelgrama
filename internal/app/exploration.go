package app

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/charle-z/pixelgrama/internal/store"
)

func (s *server) handleRandom(w http.ResponseWriter, r *http.Request) {
	item, err := s.store.RandomPublic(r.Context())
	if errors.Is(err, store.ErrPostcardNotFound) {
		s.writeError(w, http.StatusNotFound, "not_found", "no public postcards are available")
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "storage_error", "random postcard could not be selected")
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/p/%d", item.ID), http.StatusTemporaryRedirect)
}
