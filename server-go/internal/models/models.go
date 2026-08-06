// Package models defines the FlowForge domain types shared by the store,
// engine, and HTTP API. JSON tags match the contract the React UI consumes
// (camelCase), mirroring server/src/types.ts in the Node reference.
package models

type StepType = string

type WorkflowStatus = string

const (
	StatusDraft    WorkflowStatus = "draft"
	StatusApproved WorkflowStatus = "approved"
	StatusDeployed WorkflowStatus = "deployed"
)

type StepStatus = string

const (
	StepPending   StepStatus = "pending"
	StepRunning   StepStatus = "running"
	StepWaiting   StepStatus = "waiting"
	StepSucceeded StepStatus = "succeeded"
	StepFailed    StepStatus = "failed"
	StepSkipped   StepStatus = "skipped"
)

type InstanceStatus = string

const (
	InstRunning   InstanceStatus = "running"
	InstWaiting   InstanceStatus = "waiting"
	InstFailed    InstanceStatus = "failed"
	InstCompleted InstanceStatus = "completed"
	InstCancelled InstanceStatus = "cancelled"
)

// Position is the optional canvas position of a step.
type Position struct {
	X int `json:"x,omitempty"`
	Y int `json:"y,omitempty"`
}

// WorkflowStep is one step in a workflow (authoring + runtime metadata).
type WorkflowStep struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"`
	Name        string            `json:"name"`
	Params      map[string]string `json:"params"`
	Confidence  int               `json:"confidence"`
	Assumptions []string          `json:"assumptions"`
	Edited      bool              `json:"edited,omitempty"`
	Position    *Position         `json:"position,omitempty"`
}

// Workflow is a versioned, authored workflow definition.
type Workflow struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Prompt      string          `json:"prompt"`
	Status      WorkflowStatus  `json:"status"`
	Version     int             `json:"version"`
	Steps       []WorkflowStep  `json:"steps"`
	CreatedBy   string          `json:"createdBy"`
	ApprovedBy  string          `json:"approvedBy,omitempty"`
	AIModel     string          `json:"aiModel"`
	CreatedAt   string          `json:"createdAt"`
	Runs        int             `json:"runs"`
}

// StepRun is one step's runtime state within an instance.
type StepRun struct {
	StepID     string  `json:"stepId"`
	Name       string  `json:"name"`
	Type       string  `json:"type"`
	Status     string  `json:"status"`
	StartedAt  string  `json:"startedAt,omitempty"`
	DurationMs int     `json:"durationMs,omitempty"`
	Output     string  `json:"output,omitempty"`
	Note       string  `json:"note,omitempty"`
}

// Instance is one execution of a workflow.
type Instance struct {
	ID           string            `json:"id"`
	WorkflowID   string            `json:"workflowId"`
	WorkflowName string            `json:"workflowName"`
	Status       InstanceStatus    `json:"status"`
	Entity       string            `json:"entity,omitempty"`
	StartedAt    string            `json:"startedAt"`
	EndedAt      string            `json:"endedAt,omitempty"`
	Input        map[string]any    `json:"input,omitempty"`
	AutoApprove  bool              `json:"autoApprove,omitempty"`
	StepRuns     []StepRun         `json:"stepRuns"`
	CurrentStep  int               `json:"currentStep"`
	WaitingOn    string            `json:"waitingOn,omitempty"`
	Error        string            `json:"error,omitempty"`
}

// AuditEntry is one immutable audit record.
type AuditEntry struct {
	ID     string `json:"id"`
	At     string `json:"at"`
	Actor  string `json:"actor"`
	Action string `json:"action"`
	Detail string `json:"detail"`
	Kind   string `json:"kind"`
}

// MDMEntity is a master-data entity with golden records.
type MDMEntity struct {
	Key     string              `json:"key"`
	Label   string              `json:"label"`
	Icon    string              `json:"icon"`
	Fields  []string            `json:"fields"`
	Records []map[string]string `json:"records"`
}

// ControlDef is a step-type registry entry (built-in or custom).
type ControlDef struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Color       string `json:"color"`
	Icon        string `json:"icon"`
	Enabled     bool   `json:"enabled"`
	Custom      bool   `json:"custom,omitempty"`
	Description string `json:"description,omitempty"`
}

// AIConfig is the persisted AI authoring configuration.
type AIConfig struct {
	Provider string `json:"provider"`
	APIKey   string `json:"apiKey,omitempty"`
	BaseURL  string `json:"baseURL"`
	Model    string `json:"model"`
}
