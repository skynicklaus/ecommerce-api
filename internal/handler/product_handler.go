package handler

import "net/http"

func (h *V1Handler) CreateProduct(w http.ResponseWriter, r *http.Request) error {
	return WriteJSON(w, http.StatusCreated, nil)
}
