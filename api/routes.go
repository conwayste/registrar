package api

import (
	"net/http"

	"github.com/gorilla/mux"
)

func AddRoutes(router *mux.Router) {
	router.HandleFunc("/test", testRoute)
	router.HandleFunc("/servers", listServers)
	//XXX router.HandleFunc
}

func listServers(w http.ResponseWriter, r *http.Request) {
	//XXX
}

func testRoute(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"ok": true}`))
}
