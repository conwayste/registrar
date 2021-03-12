package api

import (
	"encoding/json"
	"net/http"

	"github.com/conwayste/registrar/monitor"

	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

type RouteHandler func(w http.ResponseWriter, r *http.Request, m *monitor.Monitor, log *zap.Logger) error

func AddRoutes(router *mux.Router, m *monitor.Monitor, log *zap.Logger) {
	router.HandleFunc("/servers", WithMonitorAndLog(m, log, listServers))
}

func WithMonitorAndLog(m *monitor.Monitor, log *zap.Logger, h RouteHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r, m, log); err != nil {
			w.Header().Add("content-type", "application/json")
			w.WriteHeader(http.StatusBadRequest) // TODO: get the error code from err
			w.Write([]byte(`{"ok": false}`))     // TODO: get the response body from err
		}
	}
}

type ApiError struct {
	ResponseCode int
	// ResponseBody is the response body; must support json.Marshal
	ResponseBody interface{}
	Err          error
}

//////////////////// ROUTES /////////////////////////////////

// TODO: paging
func listServers(w http.ResponseWriter, r *http.Request, m *monitor.Monitor, log *zap.Logger) error {
	serverList := m.ListServers(false)
	responseBody, err := json.Marshal(struct {
		Servers []*monitor.PublicServerInfo `json:"servers"`
	}{serverList})
	if err != nil {
		return err
	}

	w.Header().Add("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(responseBody))
	return nil
}
