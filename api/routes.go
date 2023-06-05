package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"regexp"
	"time"

	"github.com/conwayste/registrar/monitor"

	"github.com/didip/tollbooth"
	"github.com/didip/tollbooth/limiter"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

const maxAddServerBodySize = 1000 // Consider increasing if we add more fields to the /addServer request body
const maxResponseSnippetLen = 150 // Increase/decrease depending on log volume
const maxServerAddsPerSecPerIp = 10
const maxServerListsPerSecPerIp = 30

type RouteHandler func(w http.ResponseWriter, r *http.Request, m *monitor.Monitor, log *zap.Logger) error

func AddRoutes(router *mux.Router, m *monitor.Monitor, log *zap.Logger, useProxyHeaders bool) {
	maybeProxyHeaders := NoOp
	if useProxyHeaders {
		maybeProxyHeaders = handlers.ProxyHeaders
	}

	listLimiter := tollbooth.NewLimiter(maxServerListsPerSecPerIp,
		&limiter.ExpirableOptions{DefaultExpirationTTL: time.Hour})
	addLimiter := tollbooth.NewLimiter(maxServerAddsPerSecPerIp,
		&limiter.ExpirableOptions{DefaultExpirationTTL: time.Hour})
	if useProxyHeaders {
		addLimiter.SetIPLookups([]string{"X-Forwarded-For", "X-Real-IP"})
	}

	// The routes
	router.HandleFunc("/servers", maybeProxyHeaders(
		tollbooth.LimitFuncHandler(listLimiter,
			WithMonitorAndLog(m, log, listServers),
		),
	).ServeHTTP)
	router.HandleFunc("/addServer", maybeProxyHeaders(
		tollbooth.LimitFuncHandler(addLimiter,
			WithMonitorAndLog(m, log, addServer),
		),
	).ServeHTTP)
}

//////////////////// MIDDLEWARE /////////////////////////////////
func NoOp(h http.Handler) http.Handler {
	return h
}

func WithMonitorAndLog(m *monitor.Monitor, log *zap.Logger, h RouteHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r, m, log); err != nil {
			var apiErr ApiError
			if !errors.As(err, &apiErr) {
				log.Error("Uh oh, error is not wrapped in an ApiError", zap.Error(err))
				w.Header().Add("content-type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error": "Internal Server Error"}`))
				return
			}
			log := log.With(apiErr.LogData...)
			var responseSnippet string
			bodyBytes, err := json.Marshal(apiErr.ResponseBody)
			if err != nil {
				log = log.With(zap.String("marshalErr", err.Error()))
				bodyBytes = []byte(`{"error": "unknown error"}`)
			}
			responseSnippet = string(bodyBytes)
			if len(responseSnippet) > maxResponseSnippetLen {
				responseSnippet = responseSnippet[:maxResponseSnippetLen-3] + "..."
			}
			log = log.With(zap.String("responseSnippet", responseSnippet))

			if apiErr.ResponseCode/100 == 5 {
				log.Error("API error", zap.Error(apiErr.Err))
			} else if apiErr.Err != nil {
				log.Info("API issue", zap.String("issue", apiErr.Err.Error()))
			}
			w.Header().Add("content-type", "application/json")
			w.WriteHeader(apiErr.ResponseCode)
			w.Write(bodyBytes)
		}
	}
}

//////////////////// ROUTES /////////////////////////////////

func listServers(w http.ResponseWriter, r *http.Request, m *monitor.Monitor, log *zap.Logger) error {
	// TODO: middleware for following; I'm a little disappointed gorilla/mux doesn't handle this automatically
	if r.Method != http.MethodGet {
		return NewApiError(http.StatusMethodNotAllowed, `{"error": "unsupported method"}`, nil)
	}
	serverList := m.ListServers(false)

	// TODO: paging
	var truncatedResults bool
	if len(serverList) > 200 {
		serverList = serverList[:200]
		truncatedResults = true
	}

	responseBody, err := json.Marshal(struct {
		Servers          []*monitor.PublicServerInfo `json:"servers"`
		TruncatedResults bool                        `json:"truncated_results,omitempty"`
	}{serverList, truncatedResults})
	if err != nil {
		// Probably unreachable
		return err
	}

	successResponseBytes(w, responseBody)
	return nil
}

var hostAndPortRE = regexp.MustCompile(`^[^:]+:[1-9]\d*$`)

func validHostAndPort(hostAndPort string) bool {
	return hostAndPortRE.MatchString(hostAndPort)
}

func addServer(w http.ResponseWriter, r *http.Request, m *monitor.Monitor, log *zap.Logger) error {
	// TODO: middleware for following; I'm a little disappointed gorilla/mux doesn't handle this automatically
	if r.Method != http.MethodPost {
		return NewApiError(http.StatusMethodNotAllowed, `{"error": "unsupported method"}`, nil)
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
	if !validHostAndPort(serverAddr) {
		return NewApiError(http.StatusBadRequest, `{"error": "Invalid host_and_port format; expected host, then colon, then port"}`, nil)
	}

	if err := m.AddServer(serverAddr); err != nil {
		// TODO: add stats counter increment

		var serverAddErr monitor.ServerAddError
		if errors.As(err, &serverAddErr) {
			return NewApiErrorFromServerAddError(log, serverAddErr)
		}

		return NewApiError(http.StatusBadRequest, `{"error": "unknown server error; check the logs"}`, err)
	}

	successResponse(w, `{"added":true}`)
	return nil
}

type AddServerRequestBody struct {
	// HostAndPort is the public address (in "host:port" format)
	HostAndPort string `json:"host_and_port"`
}

//////////////////// ERROR HANDLING /////////////////////////////////

func successResponse(w http.ResponseWriter, body string) {
	successResponseBytes(w, []byte(body))
}

func successResponseBytes(w http.ResponseWriter, body []byte) {
	w.Header().Add("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}

type ApiError struct {
	ResponseCode int
	// ResponseBody is the response body; must support json.Marshal
	ResponseBody interface{}
	Err          error
	LogData      []zap.Field
}

func (e ApiError) Error() string {
	return e.Err.Error()
}

func (e ApiError) Unwrap() error {
	return e.Err
}

func NewApiError(code int, body string, err error, logData ...zap.Field) ApiError {
	return ApiError{
		ResponseCode: code,
		ResponseBody: []byte(body),
		Err:          err,
		LogData:      logData,
	}
}

func NewApiErrorFromServerAddError(log *zap.Logger, err monitor.ServerAddError) ApiError {
	responseCode := http.StatusInternalServerError
	errorString := "Internal Server Error" // Don't use any special characters here!
	switch err.Code {
	default:
		log.Warn("unrecognized ServerAddErrorCode", zap.Int("code", int(err.Code)))
	case monitor.ServerAddErrIsSpecialIP:
		errorString = fmt.Sprintf("IP type is not allowed")
	case monitor.ServerAddErrResolve:
		errorString = fmt.Sprintf("failed to resolve server host name")
	}
	return NewApiError(responseCode, fmt.Sprintf(`{"error":%q}`, errorString), err)
}
