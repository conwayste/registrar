package main

import (
	"context"
	glog "log"
	"net/http"
	"time"

	"github.com/conwayste/registrar/monitor"

	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

func main() {
	log, err := zap.NewDevelopment()
	if err != nil {
		glog.Fatalf("failed to construct logger: %v", err)
	}
	router := mux.NewRouter()
	router.HandleFunc("/test", testRoute)
	//router.HandleFunc
	srv := &http.Server{
		Handler: router,
		Addr:    "127.0.0.1:8000",
		// Good practice: enforce timeouts for servers you create!
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	//XXX also cancel on signals
	m := monitor.NewMonitor()
	m.AddServer("127.0.0.1:2016") //XXX hardcoded localhost server!
	go monitor.Start(ctx, log, m)

	log.Info("registrar is listening", zap.String("addr", srv.Addr))
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal("error from HTTP server", zap.Error(err))
	}
}

func testRoute(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"ok": true}`))
}
