package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/conwayste/registrar/monitor"

	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

const maxAddServerBodySize = 1000

type RouteHandler func(w http.ResponseWriter, r *http.Request, m *monitor.Monitor, log *zap.Logger) error

func AddRoutes(router *mux.Router, m *monitor.Monitor, log *zap.Logger) {
	router.HandleFunc("/servers", WithMonitorAndLog(m, log, listServers))
	router.HandleFunc("/addServer", WithMonitorAndLog(m, log, addServer))
}

func WithMonitorAndLog(m *monitor.Monitor, log *zap.Logger, h RouteHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r, m, log); err != nil {
			log.Error("error", zap.Error(err)) // TODO: consider not logging this
			w.Header().Add("content-type", "application/json")
			w.WriteHeader(http.StatusBadRequest)        // TODO: get the error code from err
			w.Write([]byte(`{"error": "bad request"}`)) // TODO: get the response body from err
		}
	}
}

//XXX use this
type ApiError struct {
	ResponseCode int
	// ResponseBody is the response body; must support json.Marshal
	ResponseBody interface{}
	Err          error
}

//////////////////// ROUTES /////////////////////////////////

// TODO: paging
func listServers(w http.ResponseWriter, r *http.Request, m *monitor.Monitor, log *zap.Logger) error {
	// TODO: middleware for following; I'm a little disappointed gorilla/mux doesn't handle this automatically
	if r.Method != http.MethodGet {
		//XXX use ApiError
		w.Header().Add("content-type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte(`{"error": "unsupported method"}`))
		return nil
	}
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

// TODO: rate limiting!!!!!!!!!!!
func addServer(w http.ResponseWriter, r *http.Request, m *monitor.Monitor, log *zap.Logger) error {
	// TODO: middleware for following; I'm a little disappointed gorilla/mux doesn't handle this automatically
	if r.Method != http.MethodPost {
		//XXX use ApiError
		w.Header().Add("content-type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte(`{"error": "unsupported method"}`))
		return nil
	}

	if r.Body == nil {
		return errors.New("request body is nil")
	}
	bodyReader := io.LimitReader(r.Body, maxAddServerBodySize)
	bodyBytes, err := ioutil.ReadAll(bodyReader)
	if err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}

	var reqBody AddServerRequestBody
	if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
		return errors.New("JSON unmarshal failure")
	}
	serverAddr := reqBody.HostAndPort
	if serverAddr == "" {
		return errors.New("invalid serverAddr")
	}
	// TODO: other checks

	if err := m.AddServer(serverAddr); err != nil {
		//XXX use ApiError
		// TODO: maybe log something or at least a stats counter increment?
		w.Header().Add("content-type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "invalid JSON"}`))
		return nil
	}

	//XXX success JSON?
	return nil
}

type AddServerRequestBody struct {
	// HostAndPort is the public address (in "host:port" format)
	HostAndPort string `json:"host_and_port"`
}
