package api

import (
	"encoding/json"
	"net/http"

	"github.com/conwayste/registrar/monitor"

	"github.com/gorilla/mux"
)

type RouteHandler func(w http.ResponseWriter, r *http.Request, m *monitor.Monitor) error

func AddRoutes(router *mux.Router, m *monitor.Monitor) {
	router.HandleFunc("/servers", WithMonitor(m, listServers))
}

func WithMonitor(m *monitor.Monitor, h RouteHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r, m); err != nil {
			w.Header().Add("content-type", "application/json")
			w.WriteHeader(http.StatusBadRequest) // TODO: get the error code from err
			w.Write([]byte(`{"ok": false}`))     // TODO: get the response body from err
		}
	}
}

func listServers(w http.ResponseWriter, r *http.Request, m *monitor.Monitor) error {
	serverList := m.ListServers()
	responseBody, err := json.Marshal(struct {
		Servers []string `json:"servers"`
	}{serverList})
	if err != nil {
		return err
	}

	w.Header().Add("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(responseBody))
	return nil
}

type ApiError struct {
	Err error
}
