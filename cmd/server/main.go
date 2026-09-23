// Команда server запускает HTTP-сервис модуля безопасности ПД.
//
// Она предоставляет POST /process (контракт маскирования/демаскирования)
// и GET /metrics.
package main

import (
	"context"
	"encoding/hex"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/duck-driven-llm-proxy-service/internal/api"
	"github.com/duck-driven-llm-proxy-service/internal/config"
	"github.com/duck-driven-llm-proxy-service/internal/detection"
	"github.com/duck-driven-llm-proxy-service/internal/metrics"
	"github.com/duck-driven-llm-proxy-service/internal/ner"
	"github.com/duck-driven-llm-proxy-service/internal/store"
)

func main() {
	cfgPath := flag.String("config", "", "путь к JSON-файлу конфигурации")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Error("загрузка конфигурации", "error", err)
		os.Exit(1)
	}

	key, err := resolveKey(cfg.AESKey)
	if err != nil {
		log.Error("разбор ключа AES", "error", err)
		os.Exit(1)
	}

	st, err := store.New(key, cfg.StoreTTL.D(), cfg.StoreMaxSize)
	if err != nil {
		log.Error("инициализация хранилища", "error", err)
		os.Exit(1)
	}

	pol, err := cfg.BuildPolicyManager()
	if err != nil {
		log.Error("построение политик", "error", err)
		os.Exit(1)
	}

	m := metrics.New()

	nerClient := ner.NewClientWithWorkers(cfg.NEREndpoint, cfg.NERWorkers)
	defer nerClient.Close()

	var det detection.Detector = ner.NewProductionDetector(nerClient)

	svc := api.NewService(det, st, pol, m)
	handler := api.NewHandler(svc, m, log, rateLimiter(cfg.MaxRPS))

	mux := http.NewServeMux()
	mux.Handle("/process", handler)
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte(m.Prometheus() + nerClient.Prometheus()))
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Info("сервер слушает", "addr", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("ошибка сервера", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	log.Info("сервер остановлен")
}

func resolveKey(hexKey string) ([]byte, error) {
	if hexKey == "" {
		return nil, os.ErrInvalid
	}
	return hex.DecodeString(hexKey)
}

// rateLimiter возвращает ограничитель в стиле token-bucket. maxRPS <= 0 отключает его.
func rateLimiter(maxRPS int) func() bool {
	if maxRPS <= 0 {
		return nil
	}
	var tokens atomic.Int64
	tokens.Store(int64(maxRPS))
	interval := time.Second / time.Duration(maxRPS)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			if tokens.Load() < int64(maxRPS) {
				tokens.Add(1)
			}
		}
	}()
	return func() bool {
		for {
			cur := tokens.Load()
			if cur <= 0 {
				return false
			}
			if tokens.CompareAndSwap(cur, cur-1) {
				return true
			}
		}
	}
}
