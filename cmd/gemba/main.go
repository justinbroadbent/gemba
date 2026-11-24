// Command gemba serves the Gemba API and web app.
//
// Environment:
//
//	GEMBA_CONFIG        path to the configuration file (default: built-in demo)
//	GEMBA_ADDR          listen address (default 127.0.0.1:8080)
//	GEMBA_REQUIRE_AUTH  set to 1 to refuse requests with no signed-in identity
//	GITHUB_TOKEN        token for workspaces with store: github
package main

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/justinbroadbent/gemba/internal/api"
	"github.com/justinbroadbent/gemba/internal/config"
	"github.com/justinbroadbent/gemba/internal/notify"
	"github.com/justinbroadbent/gemba/internal/store"
	"github.com/justinbroadbent/gemba/web"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load(os.Getenv("GEMBA_CONFIG"))
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}

	stores := map[string]store.Store{}
	for _, ws := range cfg.Workspaces {
		switch ws.Store {
		case "github":
			token := os.Getenv("GITHUB_TOKEN")
			if token == "" {
				log.Error("workspace needs GITHUB_TOKEN", "workspace", ws.Slug)
				os.Exit(1)
			}
			stores[ws.Slug] = store.NewGitHub(ws, token)
		default:
			var cells []store.DemoCell
			for _, c := range ws.Cells {
				dc := store.DemoCell{ID: c.ID}
				for _, p := range ws.People {
					if p.Cell == c.ID {
						dc.People = append(dc.People, p.Login)
					}
				}
				cells = append(cells, dc)
			}
			demo := store.NewDemo(cells, ws.Policies.ThroughputWeeks, time.Now().UTC())
			if n := len(ws.People); n > 0 {
				_ = demo.SeedTriageLead(ws.People[n/2].Login, time.Now().UTC().AddDate(0, 0, -2))
			}
			stores[ws.Slug] = demo
		}
		log.Info("workspace ready", "workspace", ws.Slug, "store", ws.Store)
	}

	notifiers := map[string]notify.Notifier{}
	for _, ws := range cfg.Workspaces {
		notifiers[ws.Slug] = notify.For(ws.Notify.TeamsWebhook)
	}
	srv := &api.Server{
		Config:      cfg,
		Stores:      stores,
		Notify:      notifiers,
		Now:         func() time.Time { return time.Now().UTC() },
		RequireAuth: os.Getenv("GEMBA_REQUIRE_AUTH") == "1",
		Static:      web.Dist(),
		Log:         log,
	}
	if cfg.Brand.Logo != "" {
		types := map[string]string{".svg": "image/svg+xml", ".png": "image/png"}
		kind, ok := types[strings.ToLower(filepath.Ext(cfg.Brand.Logo))]
		if !ok {
			log.Error("brand.logo must be an .svg or .png file", "logo", cfg.Brand.Logo)
			os.Exit(1)
		}
		if srv.Logo, err = os.ReadFile(cfg.Brand.Logo); err != nil {
			log.Error("brand.logo", "err", err)
			os.Exit(1)
		}
		srv.LogoType = kind
	}
	addr := os.Getenv("GEMBA_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	log.Info("listening", "addr", addr, "require_auth", srv.RequireAuth)
	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		log.Error("server", "err", err)
		os.Exit(1)
	}
}
