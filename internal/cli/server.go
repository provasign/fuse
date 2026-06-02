package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/provasign/fuse/internal/config"
	"github.com/provasign/fuse/internal/core"
	"github.com/provasign/fuse/internal/merge"
	"github.com/provasign/fuse/internal/parser"
)

// startServer launches the HTTP API used for programmatic merges.
func startServer(cfg *config.Config) int {
	groveClient, _ := newGrove(cfg, false)
	im := merge.New(groveClient)
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("POST /merge", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Base     string `json:"base"`
			Ours     string `json:"ours"`
			Theirs   string `json:"theirs"`
			Path     string `json:"path"`
			Language string `json:"language"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		lang := core.LanguageKey(body.Language)
		if lang == "" {
			lang = parser.DetectLanguage(body.Path, body.Ours)
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		res, err := im.Merge(ctx, []byte(body.Base), []byte(body.Ours), []byte(body.Theirs), lang, body.Path)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	fmt.Printf("fuse: serving on %s\n", addr)
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil {
		fmt.Println("fuse:", err)
		return 1
	}
	return 0
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 5<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
