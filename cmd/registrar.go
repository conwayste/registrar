package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
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

const (
	backupInterval = 15 * time.Minute
)

var (
	devMode         = flag.Bool("devMode", true, "whether to run in development mode")
	allowSpecialIPs = flag.Bool("allowSpecialIPs", false,
		"whether unusual (not global or not unicast) IPs are allowed; don't set to true in production")
	useProxyHeaders = flag.Bool("useProxyHeaders", true,
		"whether to trust X-Forwarded-For; must be true with a reverse proxy (nginx etc.); must be false otherwise")
	backupFile = flag.String("backupFile", "backup.jsonl", "backup file to save and restore to; disabled if empty")
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

	m := monitor.NewMonitor()
	if *backupFile != "" {
		go LoadFromFile(m, log, *backupFile)
	}
	m.AllowSpecialIPs = *allowSpecialIPs

	conn, err := net.ListenPacket("udp", "0.0.0.0:0")
	if err != nil {
		log.Error("failed to open UDP port", zap.Error(err))
		return
	}
	defer conn.Close()

	grp, grpCtx := errgroup.WithContext(ctx) // grpCtx is cancelled once either returns non-nil error or both exit
	grp.Go(func() error {
		return m.Send(grpCtx, log, conn)
	})
	grp.Go(func() error {
		return m.Receive(grpCtx, log, conn)
	})
	if *backupFile != "" {
		grp.Go(func() error {
			return BackupToFile(grpCtx, m, log, *backupFile)
		})
	}

	router := mux.NewRouter()
	api.AddRoutes(router, m, log, *useProxyHeaders)
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
			log.Error("error from Send, Receive, or Backup", zap.Error(err))
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

type BackedUpServer struct {
	Addr string `json"addr"`
}

// LoadFromFile restores a backup. Since it resolves each server serially, it can be slow.
func LoadFromFile(m *monitor.Monitor, log *zap.Logger, path string) {
	t := time.Now()
	defer func() {
		log.Info("LoadFromFile finished", zap.Duration("duration", time.Since(t)))
	}()
	f, err := os.Open(path)
	if err != nil {
		// TODO: if the error is due to the file not existing, it shouldn't be logged as an error
		log.Error("failed to open backup file for loading", zap.Error(err))
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Split(bufio.ScanLines)
	for i := 0; scanner.Scan(); i++ {
		var b BackedUpServer
		if err := json.Unmarshal(scanner.Bytes(), &b); err != nil {
			log.Error("failed to unmarshal line", zap.Error(err), zap.Int("lineNo", i+1))
			break
		}
		m.AddServer(b.Addr)
	}
	if err := scanner.Err(); err != nil {
		log.Error("error while reading lines from backup file", zap.Error(err))
	}
}

// BackupToFile periodically backs up the server list to a file (safely)
func BackupToFile(ctx context.Context, m *monitor.Monitor, log *zap.Logger, path string) error {
	ticker := time.NewTicker(backupInterval)
	for {
		select {
		// See if context completed
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Handled below
		}

		tempPath := fmt.Sprintf(".%s.new%d", path, time.Now().UnixNano())
		f, err := os.OpenFile(tempPath, os.O_RDWR|os.O_CREATE, 0755)
		if err != nil {
			log.Error("failed to open backup file for writing", zap.String("tempPath", tempPath), zap.Error(err))
			continue
		}
		addrs := m.ListServerAddresses()
		for _, addr := range addrs {
			b := BackedUpServer{
				Addr: addr,
			}
			line, err := json.Marshal(&b)
			if err != nil {
				log.Error("marshal BackedUpServer", zap.Error(err))
				continue
			}
			line = append(line, '\n')
			f.Write(line)
		}
		if err := f.Close(); err != nil {
			log.Error("failed to close backup file")
			// Don't overwrite the good backup file!
			continue
		}
		if err := os.Rename(tempPath, path); err != nil {
			log.Error("failed to move backup file from temp location to perm. loc.", zap.Error(err))
			continue
		}
	}
}
