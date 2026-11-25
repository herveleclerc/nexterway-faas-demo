package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/herveleclerc/nexterway-faas-demo/src/demo-complete/web"
)

type Config struct {
	Port       string
	RouterURL  string
	TenantHost string
	PodName    string
}

func loadConfig() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	routerURL := os.Getenv("FISSION_ROUTER_URL")
	if routerURL == "" {
		// Service Fission Router interne
		routerURL = "http://router.fission"
	}

	tenantHost := os.Getenv("TENANT_HOST")
	if tenantHost == "" {
		tenantHost = "wy-3a1fd37f-1-0-0.nexterway.caas.fr"
	}

	podName := os.Getenv("HOSTNAME")
	if podName == "" {
		podName = "local-frontend-dev"
	}

	return Config{
		Port:       port,
		RouterURL:  routerURL,
		TenantHost: tenantHost,
		PodName:    podName,
	}
}

func main() {
	cfg := loadConfig()
	log.Printf("Démarrage Frontend BFF Nexterway sur :%s (Fission Router: %s, TenantHost: %s)", cfg.Port, cfg.RouterURL, cfg.TenantHost)

	tmpl, err := template.ParseFS(web.Content, "templates/index.html")
	if err != nil {
		log.Fatalf("Erreur chargement template: %v", err)
	}

	mux := http.NewServeMux()

	// 1. Fichiers statiques (CSS, JS) embarqués
	staticFS := http.FileServer(http.FS(web.Content))
	mux.Handle("/static/", staticFS)

	// 2. Page d'accueil Dashboard
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		data := map[string]string{
			"PodName":    cfg.PodName,
			"RouterURL":  cfg.RouterURL,
			"TenantHost": cfg.TenantHost,
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, data); err != nil {
			log.Printf("Erreur rendering template: %v", err)
			http.Error(w, "Erreur interne", http.StatusInternalServerError)
		}
	})

	// 3. Probes Kubernetes
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "OK")
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "READY")
	})

	// 4. Proxy BFF vers la fonction Fission /api/tasks
	parsedRouterURL, err := url.Parse(cfg.RouterURL)
	if err != nil {
		log.Fatalf("URL Fission Router invalide: %v", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(parsedRouterURL)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		// Le routeur Fission exige le header Host du tenant pour faire correspondre le trigger HTTP !
		if cfg.TenantHost != "" {
			req.Host = cfg.TenantHost
			req.Header.Set("Host", cfg.TenantHost)
		}
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("[PROXY ERROR] %s %s -> Fission Router (%s) échoué: %v", r.Method, r.URL.String(), cfg.RouterURL, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  "error",
			"code":    502,
			"message": fmt.Sprintf("Erreur de connexion au routeur Fission (%s) : %v. Vérifiez que router.fission est joignable.", cfg.RouterURL, err),
		})
	}

	proxy.ModifyResponse = func(resp *http.Response) error {
		log.Printf("[PROXY] Fission Router a répondu: HTTP %d pour %s %s", resp.StatusCode, resp.Request.Method, resp.Request.URL.Path)
		return nil
	}

	mux.HandleFunc("/api/tasks", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[PROXY INCOMING] %s %s (Host entrant: %s, Host transmis: %s)", r.Method, r.URL.RequestURI(), r.Host, cfg.TenantHost)
		proxy.ServeHTTP(w, r)
	})

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Gestion du graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Erreur serveur: %v", err)
		}
	}()

	<-stop
	log.Println("Arrêt gracieux du frontend...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
	log.Println("Frontend arrêté.")
}
