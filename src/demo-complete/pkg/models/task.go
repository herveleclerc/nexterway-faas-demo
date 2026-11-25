package models

import (
	"encoding/json"
	"time"
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
