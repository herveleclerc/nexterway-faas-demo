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
)

// TaskStatus représente l'état d'avancement d'une tâche
type TaskStatus string

const (
	StatusPending    TaskStatus = "PENDING"
	StatusProcessing TaskStatus = "PROCESSING"
	StatusCompleted  TaskStatus = "COMPLETED"
	StatusFailed     TaskStatus = "FAILED"
)

// TaskPriority définit le niveau de priorité
type TaskPriority string

const (
	PriorityLow    TaskPriority = "LOW"
	PriorityMedium TaskPriority = "MEDIUM"
	PriorityHigh   TaskPriority = "HIGH"
	PriorityUrgent TaskPriority = "URGENT"
)

// Task est l'entité stockée dans la base PostgreSQL CloudNativePG
type Task struct {
	ID          string          `json:"id"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Priority    TaskPriority    `json:"priority"`
	Status      TaskStatus      `json:"status"`
	Result      json.RawMessage `json:"result,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// CreateTaskRequest est le payload envoyé pour créer une tâche
type CreateTaskRequest struct {
	Title       string       `json:"title"`
	Description string       `json:"description"`
	Priority    TaskPriority `json:"priority"`
}

// ProcessTaskRequest est le payload optionnel pour déclencher un traitement
type ProcessTaskRequest struct {
	WorkerID string `json:"worker_id,omitempty"`
	Notes    string `json:"notes,omitempty"`
}

// TaskStats regroupe les compteurs d'état pour le dashboard
type TaskStats struct {
	Total      int `json:"total"`
	Pending    int `json:"pending"`
	Processing int `json:"processing"`
	Completed  int `json:"completed"`
	Failed     int `json:"failed"`
}

// TasksResponse est le format standard retourné par la fonction
type TasksResponse struct {
	Status  string     `json:"status"`
	Message string     `json:"message,omitempty"`
	Stats   *TaskStats `json:"stats,omitempty"`
	Data    any        `json:"data,omitempty"`
	PodName string     `json:"pod_name,omitempty"`
}

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
				strings.ReplaceAll(envDBURL, "pg-cluster", "pg-cluster-rw.default.svc.cluster.local"),
				strings.ReplaceAll(envDBURL, "pg-cluster", "pg-cluster-rw.demo-fission-1.svc.cluster.local"),
				strings.ReplaceAll(envDBURL, "pg-cluster", "pg-cluster-rw.wy-3a1fd37f-1-0-0.svc.cluster.local"),
				strings.ReplaceAll(envDBURL, "pg-cluster", "pg-cluster-rw.postgres.svc.cluster.local"),
				strings.ReplaceAll(envDBURL, "pg-cluster", "pg-demo-rw.demo-fission-1.svc.cluster.local"),
				strings.ReplaceAll(envDBURL, "pg-cluster", "pg-demo-rw.default.svc.cluster.local"),
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
		"pg-demo-rw.default.svc.cluster.local",
		"pg-demo-rw",
		"pg-cluster-rw.default.svc.cluster.local",
		"pg-cluster-rw.demo-fission-1.svc.cluster.local",
		"pg-cluster-rw.wy-3a1fd37f-1-0-0.svc.cluster.local",
		"pg-cluster-rw.postgres.svc.cluster.local",
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

	// Si déjà connecté, on réutilise le pool thread-safe existant (évite les blocages Ping)
	if db != nil {
		return db, nil
	}

	candidates := buildCandidateDSNs()
	var errorReports []string

	for _, dsn := range candidates {
		displayDSN := maskDSN(dsn)

		database, err := sql.Open("postgres", dsn)
		if err != nil {
			errorReports = append(errorReports, fmt.Sprintf("[%s] open err: %v", displayDSN, err))
			continue
		}

		database.SetMaxOpenConns(10)
		database.SetMaxIdleConns(5)
		database.SetConnMaxLifetime(5 * time.Minute)

		if err := database.Ping(); err != nil {
			_ = database.Close()
			errorReports = append(errorReports, fmt.Sprintf("[%s] ping err: %v", displayDSN, err))
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

	return nil, fmt.Errorf("aucune cible PostgreSQL joignable. Détails par cible : %s",
		strings.Join(errorReports, " | "))
}

func maskDSN(dsn string) string {
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
		sendJSON(w, http.StatusServiceUnavailable, TasksResponse{
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
		sendJSON(w, http.StatusMethodNotAllowed, TasksResponse{
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
		sendJSON(w, http.StatusInternalServerError, TasksResponse{
			Status:  "error",
			Message: fmt.Sprintf("Erreur lecture tâches: %v", err),
		})
		return
	}
	defer rows.Close()

	tasks := make([]Task, 0)
	stats := &TaskStats{}

	for rows.Next() {
		var t Task
		var rawResult []byte
		if err := rows.Scan(&t.ID, &t.Title, &t.Description, &t.Priority, &t.Status, &rawResult, &t.CreatedAt, &t.UpdatedAt); err != nil {
			log.Printf("Erreur scan: %v", err)
			continue
		}
		t.Result = rawResult
		tasks = append(tasks, t)

		stats.Total++
		switch t.Status {
		case StatusPending:
			stats.Pending++
		case StatusProcessing:
			stats.Processing++
		case StatusCompleted:
			stats.Completed++
		case StatusFailed:
			stats.Failed++
		}
	}

	sendJSON(w, http.StatusOK, TasksResponse{
		Status:  "success",
		Stats:   stats,
		Data:    tasks,
		PodName: os.Getenv("HOSTNAME"),
	})
}

func handleCreateTask(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	var req CreateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSON(w, http.StatusBadRequest, TasksResponse{
			Status:  "error",
			Message: "Payload JSON invalide",
		})
		return
	}

	if strings.TrimSpace(req.Title) == "" {
		sendJSON(w, http.StatusBadRequest, TasksResponse{
			Status:  "error",
			Message: "Le champ 'title' est obligatoire",
		})
		return
	}

	if req.Priority == "" {
		req.Priority = PriorityMedium
	}

	task := Task{
		ID:          generateUUID(),
		Title:       req.Title,
		Description: req.Description,
		Priority:    req.Priority,
		Status:      StatusPending,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	_, err := db.Exec(`
		INSERT INTO tasks (id, title, description, priority, status, result, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, '{}'::jsonb, $6, $7)
	`, task.ID, task.Title, task.Description, task.Priority, task.Status, task.CreatedAt, task.UpdatedAt)
	if err != nil {
		sendJSON(w, http.StatusInternalServerError, TasksResponse{
			Status:  "error",
			Message: fmt.Sprintf("Erreur insertion PostgreSQL: %v", err),
		})
		return
	}

	sendJSON(w, http.StatusCreated, TasksResponse{
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
		sendJSON(w, http.StatusBadRequest, TasksResponse{
			Status:  "error",
			Message: "Identifiant de tâche 'id' manquant",
		})
		return
	}

	res, err := db.Exec(`
		UPDATE tasks
		SET status = $1, updated_at = $2
		WHERE id = $3 AND status = $4
	`, StatusProcessing, time.Now().UTC(), taskID, StatusPending)
	if err != nil {
		sendJSON(w, http.StatusInternalServerError, TasksResponse{
			Status:  "error",
			Message: fmt.Sprintf("Erreur mise à jour statut: %v", err),
		})
		return
	}

	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		sendJSON(w, http.StatusConflict, TasksResponse{
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
	`, StatusCompleted, resultBytes, time.Now().UTC(), taskID)
	if err != nil {
		sendJSON(w, http.StatusInternalServerError, TasksResponse{
			Status:  "error",
			Message: fmt.Sprintf("Erreur sauvegarde résultat PostgreSQL: %v", err),
		})
		return
	}

	sendJSON(w, http.StatusOK, TasksResponse{
		Status:  "success",
		Message: fmt.Sprintf("Tâche %s exécutée et enregistrée avec succès", taskID),
		Data:    resultPayload,
		PodName: os.Getenv("HOSTNAME"),
	})
}

func sendJSON(w http.ResponseWriter, code int, resp TasksResponse) {
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
