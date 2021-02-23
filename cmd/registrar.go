package main

import (
	"context"
	glog "log"
	"net"
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

	conn, err := net.ListenPacket("udp", "0.0.0.0:0")
	if err != nil {
		log.Error("failed to open UDP port", zap.Error(err))
		return
	}
	defer conn.Close()

	m := monitor.NewMonitor()
	if err := m.AddServer("chococat.conwayste.rs:2016"); err != nil { //XXX hardcoded remote server!
		log.Error("failed to add server", zap.Error(err))
		return
	}
	if err := m.AddServer("127.0.0.1:2016"); err != nil { //XXX hardcoded localhost server!
		log.Error("failed to add server", zap.Error(err))
		return
	}
	go monitor.Send(ctx, log, m, conn)
	go monitor.Receive(ctx, log, m, conn)
	//XXX errgroup for above

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	log.Info("registrar is listening", zap.String("httpAddr", srv.Addr), zap.String("udpAddr", localAddr.String()))
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal("error from HTTP server", zap.Error(err))
	}
}

func testRoute(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"ok": true}`))
}
