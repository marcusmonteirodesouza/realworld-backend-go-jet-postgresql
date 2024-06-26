package main

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
)

func (app *application) healthcheck(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	err := app.db.PingContext(r.Context())
	if err != nil {
		app.writeErrorResponse(w, err)
		return
	}

	err = writeJSON(w, http.StatusOK, envelope{})
	if err != nil {
		app.writeErrorResponse(w, err)
	}
}
