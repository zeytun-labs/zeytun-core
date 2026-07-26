package clashapi

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
)

func tempRulesRouter(server *Server) http.Handler {
	r := chi.NewRouter()
	r.Put("/", putTempRules(server))
	return r
}

func permanentRulesRouter(server *Server) http.Handler {
	r := chi.NewRouter()
	r.Put("/", putPermanentRules(server))
	return r
}

func liveRulesRouter(server *Server) http.Handler {
	r := chi.NewRouter()
	r.Put("/", putLiveRules(server))
	return r
}

func putTempRules(server *Server) func(w http.ResponseWriter, r *http.Request) {
	return putLiveJSON(server, func(body []byte) error {
		return server.router.ReplaceTempRulesJSON(body)
	}, "expected JSON array of temp rules")
}

func putPermanentRules(server *Server) func(w http.ResponseWriter, r *http.Request) {
	return putLiveJSON(server, func(body []byte) error {
		return server.router.ReplacePermanentRulesJSON(body)
	}, "expected JSON array of permanent rules")
}

func putLiveRules(server *Server) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, ErrBadRequest)
			return
		}
		if err := server.router.ReplaceLiveRulesJSON(body); err != nil {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, newError(err.Error()))
			return
		}
		render.NoContent(w, r)
	}
}

func putLiveJSON(server *Server, apply func([]byte) error, badMsg string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, ErrBadRequest)
			return
		}
		if len(body) > 0 && string(body) != "null" {
			var probe []json.RawMessage
			if err := json.Unmarshal(body, &probe); err != nil {
				render.Status(r, http.StatusBadRequest)
				render.JSON(w, r, newError(badMsg))
				return
			}
		}
		if err := apply(body); err != nil {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, newError(err.Error()))
			return
		}
		render.NoContent(w, r)
	}
}
