package main

import (
	"context"
	"flag"
	glog "log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/conwayste/registrar/api"
	"github.com/conwayste/registrar/monitor"

	"github.com/gorilla/mux"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

var (
	// TODO: logLevel = flag.String("logLevel", "debug", "error|warn|info|debug")
	devMode = flag.Bool("devMode", true, "whether to run in development mode")
)

func main() {
	flag.Parse() // required to get above vars set to correct values
	var err error
	var log *zap.Logger
	if *devMode {
		log, err = zap.NewDevelopment()
	} else {
		log, err = zap.NewProduction()
	}
	if err != nil {
		glog.Fatalf("failed to construct logger: %v", err)
	}
	defer log.Sync() // Must be called on shutdown

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	// Cancel on SIGINT (Ctrl-C)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		<-sigCh
		log.Info("SIGINT received; cancelling...")
		cancelFunc()
	}()

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
	grp, grpCtx := errgroup.WithContext(ctx) // grpCtx is cancelled once either returns non-nil error or both exit
	grp.Go(func() error {
		return m.Send(grpCtx, log, conn)
	})
	grp.Go(func() error {
		return m.Receive(grpCtx, log, conn)
	})

	router := mux.NewRouter()
	api.AddRoutes(router, m, log)
	srv := &http.Server{
		Handler: router,
		Addr:    "127.0.0.1:8000",
		// Good practice: enforce timeouts for servers you create!
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	go func() {
		err := grp.Wait()
		if err != nil && err != context.Canceled {
			log.Error("error from Send or Receive", zap.Error(err))
		}
		log.Info("errgroup exited; shutting down HTTP server...")
		srv.Shutdown(ctx)
	}()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	log.Info("registrar is listening", zap.String("httpAddr", srv.Addr), zap.String("udpAddr", localAddr.String()))
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal("error from HTTP server", zap.Error(err))
	}
}
