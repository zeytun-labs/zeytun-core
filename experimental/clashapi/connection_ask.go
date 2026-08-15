package clashapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
)

func connectionAskRouter(server *Server) http.Handler {
	r := chi.NewRouter()
	r.Post("/decide", decideConnectionAsk(server))
	r.Post("/forget", forgetAskSession(server))
	return r
}

type decideConnectionAskRequest struct {
	ID       string `json:"id"`
	Outbound string `json:"outbound"`
	// action: route | reject | final | direct | block
	Action string `json:"action"`
	Reject bool   `json:"reject"`
}

func decideConnectionAsk(server *Server) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var body decideConnectionAskRequest
		if err := render.DecodeJSON(r.Body, &body); err != nil {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, ErrBadRequest)
			return
		}
		if body.ID == "" {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, newError("id required"))
			return
		}
		reject := body.Reject || body.Action == "reject" || body.Action == "block"
		outbound := body.Outbound
		if body.Action == "final" || body.Action == "timeout" {
			outbound = ""
			reject = false
		}
		if body.Action == "direct" && outbound == "" {
			outbound = "direct"
		}
		if err := server.router.DecideConnectionAsk(body.ID, outbound, reject); err != nil {
			render.Status(r, http.StatusNotFound)
			render.JSON(w, r, newError(err.Error()))
			return
		}
		render.NoContent(w, r)
	}
}

type forgetAskSessionRequest struct {
	Keys []string `json:"keys"`
}

func forgetAskSession(server *Server) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var body forgetAskSessionRequest
		if err := render.DecodeJSON(r.Body, &body); err != nil {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, ErrBadRequest)
			return
		}
		server.router.ForgetAskSessionKeys(body.Keys)
		render.NoContent(w, r)
	}
}
