package main

import (
	cryptorand "crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	_ "github.com/lib/pq"

	"github.com/herveleclerc/nexterway-faas-demo/src/demo-complete/pkg/models"
)

var (
	db   *sql.DB
	dbMu sync.Mutex
)

// generateUUID génère un identifiant UUID v4 standard
func generateUUID() string {
	b := make([]byte, 16)
	_, _ = cryptorand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// buildCandidateDSNs génère la liste ordonnée des DSNs possibles
func buildCandidateDSNs() []string {
	var candidates []string

	envDBURL := os.Getenv("DATABASE_URL")
	port := os.Getenv("DB_PORT")
	if port == "" {
		port = "5432"
	}
	user := os.Getenv("DB_USER")
	if user == "" {
		user = "taskuser"
	}
	password := os.Getenv("DB_PASSWORD")
	dbname := os.Getenv("DB_NAME")
	if dbname == "" {
		dbname = "taskdb"
	}
	sslmode := os.Getenv("DB_SSLMODE")
	if sslmode == "" {
		sslmode = "prefer"
	}

	// 1. Si DATABASE_URL est fournie
	if envDBURL != "" {
		candidates = append(candidates, envDBURL)

		// Si DATABASE_URL contient "pg-cluster", CloudNativePG crée en réalité le service <cluster>-rw
		if strings.Contains(envDBURL, "pg-cluster") {
			candidates = append(candidates,
				strings.ReplaceAll(envDBURL, "pg-cluster", "pg-cluster-rw"),
				strings.ReplaceAll(envDBURL, "pg-cluster", "pg-cluster-rw.demo-fission-1.svc.cluster.local"),
				strings.ReplaceAll(envDBURL, "pg-cluster", "pg-cluster-rw.wy-3a1fd37f-1-0-0.svc.cluster.local"),
				strings.ReplaceAll(envDBURL, "pg-cluster", "pg-demo-rw.demo-fission-1.svc.cluster.local"),
			)
		}
	}

	// 2. Si DB_HOST est explicitement spécifié
	if explicitHost := os.Getenv("DB_HOST"); explicitHost != "" {
		dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s connect_timeout=3",
			explicitHost, port, user, password, dbname, sslmode)
		candidates = append([]string{dsn}, candidates...)
	}

	// 3. Fallbacks standards pour CloudNativePG (services -rw créés par l'opérateur)
	fallbackHosts := []string{
		"pg-demo-rw.demo-fission-1.svc.cluster.local",
		"pg-demo-rw",
		"pg-cluster-rw.demo-fission-1.svc.cluster.local",
		"pg-cluster-rw.wy-3a1fd37f-1-0-0.svc.cluster.local",
		"pg-cluster-rw",
	}

	for _, h := range fallbackHosts {
		candidates = append(candidates, fmt.Sprintf(
			"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s connect_timeout=3",
			h, port, user, password, dbname, sslmode,
		))
	}

	return candidates
}

// initDB initialise et teste la connexion au cluster PostgreSQL CloudNativePG
func initDB() (*sql.DB, error) {
	dbMu.Lock()
	defer dbMu.Unlock()

	// Si déjà connecté et actif
	if db != nil {
		if err := db.Ping(); err == nil {
			return db, nil
		}
		_ = db.Close()
		db = nil
	}

	candidates := buildCandidateDSNs()
	var lastErr error
	var attempted []string

	for _, dsn := range candidates {
		// Masquage du mot de passe pour les logs
		displayDSN := maskDSN(dsn)
		attempted = append(attempted, displayDSN)

		database, err := sql.Open("postgres", dsn)
		if err != nil {
			lastErr = err
			continue
		}

		database.SetMaxOpenConns(10)
		database.SetMaxIdleConns(5)
		database.SetConnMaxLifetime(5 * time.Minute)

		if err := database.Ping(); err != nil {
			_ = database.Close()
			lastErr = err
			continue
		}

		// Connexion réussie !
		log.Printf("[FISSION] Connexion PostgreSQL CloudNativePG établie avec succès via : %s", displayDSN)

		// Création idempotente de la table des tâches
		createTableSQL := `
		CREATE TABLE IF NOT EXISTS tasks (
			id VARCHAR(36) PRIMARY KEY,
			title VARCHAR(255) NOT NULL,
			description TEXT,
			priority VARCHAR(20) NOT NULL DEFAULT 'MEDIUM',
			status VARCHAR(20) NOT NULL DEFAULT 'PENDING',
			result JSONB,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
		CREATE INDEX IF NOT EXISTS idx_tasks_created_at ON tasks(created_at DESC);
		`
		if _, err := database.Exec(createTableSQL); err != nil {
			log.Printf("[FISSION] Note init table: %v", err)
		}

		db = database
		return db, nil
	}

	return nil, fmt.Errorf("aucune cible PostgreSQL joignable (Dernière erreur: %v). Cibles testées : %s",
		lastErr, strings.Join(attempted, " | "))
}

func maskDSN(dsn string) string {
	// Masque le password pour éviter d'exposer des secrets dans les logs
	if idx := strings.Index(dsn, "password="); idx != -1 {
		end := strings.Index(dsn[idx:], " ")
		if end != -1 {
			return dsn[:idx] + "password=***" + dsn[idx+end:]
		}
		return dsn[:idx] + "password=***"
	}
	return dsn
}

// Handler est le point d'entrée universel pour Fission
func Handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	database, err := initDB()
	if err != nil {
		sendJSON(w, http.StatusServiceUnavailable, models.TasksResponse{
			Status:  "error",
			Message: fmt.Sprintf("Connexion PostgreSQL indisponible : %v", err),
			PodName: os.Getenv("HOSTNAME"),
		})
		return
	}

	action := r.URL.Query().Get("action")
	taskID := r.URL.Query().Get("id")

	switch {
	case r.Method == http.MethodGet:
		handleListTasks(w, database)

	case r.Method == http.MethodPost && (action == "process" || strings.HasSuffix(r.URL.Path, "/process")):
		handleProcessTask(w, r, database, taskID)

	case r.Method == http.MethodPost:
		handleCreateTask(w, r, database)

	default:
		sendJSON(w, http.StatusMethodNotAllowed, models.TasksResponse{
			Status:  "error",
			Message: fmt.Sprintf("Méthode %s non supportée", r.Method),
		})
	}
}

func handleListTasks(w http.ResponseWriter, db *sql.DB) {
	rows, err := db.Query(`
		SELECT id, title, description, priority, status, COALESCE(result, '{}'::jsonb), created_at, updated_at
		FROM tasks
		ORDER BY created_at DESC
		LIMIT 100
	`)
	if err != nil {
		sendJSON(w, http.StatusInternalServerError, models.TasksResponse{
			Status:  "error",
			Message: fmt.Sprintf("Erreur lecture tâches: %v", err),
		})
		return
	}
	defer rows.Close()

	tasks := make([]models.Task, 0)
	stats := &models.TaskStats{}

	for rows.Next() {
		var t models.Task
		var rawResult []byte
		if err := rows.Scan(&t.ID, &t.Title, &t.Description, &t.Priority, &t.Status, &rawResult, &t.CreatedAt, &t.UpdatedAt); err != nil {
			log.Printf("Erreur scan: %v", err)
			continue
		}
		t.Result = rawResult
		tasks = append(tasks, t)

		stats.Total++
		switch t.Status {
		case models.StatusPending:
			stats.Pending++
		case models.StatusProcessing:
			stats.Processing++
		case models.StatusCompleted:
			stats.Completed++
		case models.StatusFailed:
			stats.Failed++
		}
	}

	sendJSON(w, http.StatusOK, models.TasksResponse{
		Status:  "success",
		Stats:   stats,
		Data:    tasks,
		PodName: os.Getenv("HOSTNAME"),
	})
}

func handleCreateTask(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	var req models.CreateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSON(w, http.StatusBadRequest, models.TasksResponse{
			Status:  "error",
			Message: "Payload JSON invalide",
		})
		return
	}

	if strings.TrimSpace(req.Title) == "" {
		sendJSON(w, http.StatusBadRequest, models.TasksResponse{
			Status:  "error",
			Message: "Le champ 'title' est obligatoire",
		})
		return
	}

	if req.Priority == "" {
		req.Priority = models.PriorityMedium
	}

	task := models.Task{
		ID:          generateUUID(),
		Title:       req.Title,
		Description: req.Description,
		Priority:    req.Priority,
		Status:      models.StatusPending,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	_, err := db.Exec(`
		INSERT INTO tasks (id, title, description, priority, status, result, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, '{}'::jsonb, $6, $7)
	`, task.ID, task.Title, task.Description, task.Priority, task.Status, task.CreatedAt, task.UpdatedAt)
	if err != nil {
		sendJSON(w, http.StatusInternalServerError, models.TasksResponse{
			Status:  "error",
			Message: fmt.Sprintf("Erreur insertion PostgreSQL: %v", err),
		})
		return
	}

	sendJSON(w, http.StatusCreated, models.TasksResponse{
		Status:  "success",
		Message: "Tâche créée avec succès dans PostgreSQL",
		Data:    task,
		PodName: os.Getenv("HOSTNAME"),
	})
}

func handleProcessTask(w http.ResponseWriter, r *http.Request, db *sql.DB, taskID string) {
	if taskID == "" {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		taskID = body["id"]
	}

	if taskID == "" {
		sendJSON(w, http.StatusBadRequest, models.TasksResponse{
			Status:  "error",
			Message: "Identifiant de tâche 'id' manquant",
		})
		return
	}

	res, err := db.Exec(`
		UPDATE tasks
		SET status = $1, updated_at = $2
		WHERE id = $3 AND status = $4
	`, models.StatusProcessing, time.Now().UTC(), taskID, models.StatusPending)
	if err != nil {
		sendJSON(w, http.StatusInternalServerError, models.TasksResponse{
			Status:  "error",
			Message: fmt.Sprintf("Erreur mise à jour statut: %v", err),
		})
		return
	}

	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		sendJSON(w, http.StatusConflict, models.TasksResponse{
			Status:  "error",
			Message: "Tâche introuvable ou déjà traitée",
		})
		return
	}

	start := time.Now()
	time.Sleep(time.Duration(100+rand.Intn(200)) * time.Millisecond)
	duration := time.Since(start)

	resultPayload := map[string]any{
		"execution_duration_ms": duration.Milliseconds(),
		"processed_at":          time.Now().UTC().Format(time.RFC3339),
		"fission_pod":           os.Getenv("HOSTNAME"),
		"executor_engine":       "fission-go-1.22",
		"metrics": map[string]any{
			"cpu_usage_score":  rand.Intn(100),
			"database_latency": "1.2ms",
			"validated":        true,
		},
	}
	resultBytes, _ := json.Marshal(resultPayload)

	_, err = db.Exec(`
		UPDATE tasks
		SET status = $1, result = $2, updated_at = $3
		WHERE id = $4
	`, models.StatusCompleted, resultBytes, time.Now().UTC(), taskID)
	if err != nil {
		sendJSON(w, http.StatusInternalServerError, models.TasksResponse{
			Status:  "error",
			Message: fmt.Sprintf("Erreur sauvegarde résultat PostgreSQL: %v", err),
		})
		return
	}

	sendJSON(w, http.StatusOK, models.TasksResponse{
		Status:  "success",
		Message: fmt.Sprintf("Tâche %s exécutée et enregistrée avec succès", taskID),
		Data:    resultPayload,
		PodName: os.Getenv("HOSTNAME"),
	})
}

func sendJSON(w http.ResponseWriter, code int, resp models.TasksResponse) {
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(resp)
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8888"
	}
	log.Printf("Démarrage du service de tâches FaaS autonome sur :%s ...", port)
	http.HandleFunc("/", Handler)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Serveur arrêté: %v", err)
	}
}
