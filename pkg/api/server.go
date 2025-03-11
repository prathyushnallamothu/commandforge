package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/prathyushnallamothu/commandforge/pkg/executor"
	"github.com/prathyushnallamothu/commandforge/pkg/flow"
)

// Server represents the API server
type Server struct {
	Router       *mux.Router
	FlowManager  *flow.FlowManager
	Addr         string
	Clients      map[string][]*websocket.Conn
	ClientsMutex sync.Mutex
	upgrader     websocket.Upgrader
}

// CommandRequest represents a request to execute a command
type CommandRequest struct {
	Command string `json:"command"`
}

// CommandResponse represents a response from executing a command
type CommandResponse struct {
	Success   bool   `json:"success"`
	CommandID string `json:"command_id,omitempty"`
	Output    string `json:"output,omitempty"`
	Error     string `json:"error,omitempty"`
}

// CommandStatusResponse represents the status of a command
type CommandStatusResponse struct {
	Running     bool     `json:"running"`
	ExitCode    int      `json:"exit_code"`
	Output      string   `json:"output,omitempty"`
	Error       string   `json:"error,omitempty"`
	OutputList  []string `json:"output_list,omitempty"`
	ErrorList   []string `json:"error_list,omitempty"`
	Duration    float64  `json:"duration"`
	Incremental bool     `json:"incremental,omitempty"` // Whether this is an incremental update
	Complete    bool     `json:"complete,omitempty"`    // Whether this is the final update
}

// NewServer creates a new API server
func NewServer(addr string, flowManager *flow.FlowManager) *Server {
	router := mux.NewRouter()

	server := &Server{
		Router:      router,
		FlowManager: flowManager,
		Addr:        addr,
		Clients:     make(map[string][]*websocket.Conn),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for now
			},
		},
	}

	// Register routes
	server.registerRoutes()

	return server
}

// registerRoutes registers all API routes
func (s *Server) registerRoutes() {
	// API version prefix
	api := s.Router.PathPrefix("/api/v1").Subrouter()

	// Health check endpoint
	api.HandleFunc("/health", s.healthCheckHandler).Methods("GET")

	// Flow management endpoints
	api.HandleFunc("/flows", s.createFlowHandler).Methods("POST")
	api.HandleFunc("/flows/{id}", s.getFlowHandler).Methods("GET")

	// Command execution endpoints
	api.HandleFunc("/flows/{id}/execute", s.executeCommandHandler).Methods("POST")
	api.HandleFunc("/flows/{id}/commands/{command_id}", s.getCommandStatusHandler).Methods("GET")

	// Streaming endpoints
	api.HandleFunc("/flows/{id}/stream", s.streamFlowHandler)
	api.HandleFunc("/flows/{id}/commands/{command_id}/stream", s.streamCommandHandler)
}

// Start starts the API server
func (s *Server) Start() error {
	log.Printf("Starting API server on %s", s.Addr)
	return http.ListenAndServe(s.Addr, s.Router)
}

// healthCheckHandler handles health check requests
func (s *Server) healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// createFlowHandler creates a new flow
func (s *Server) createFlowHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Parse request body
	var request struct {
		Type string `json:"type"`
		Goal string `json:"goal"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Create flow based on type
	var flowID string
	var err error

	ctx := r.Context()

	// Generate a unique flow ID
	flowID = fmt.Sprintf("flow-%d", time.Now().UnixNano())

	// Create flow based on type
	var flowType flow.FlowType
	switch request.Type {
	case "planning":
		flowType = flow.FlowTypePlanning
	default:
		http.Error(w, "Unsupported flow type", http.StatusBadRequest)
		return
	}

	// Create the flow
	_, err = s.FlowManager.CreateFlow(ctx, flowType, flowID)

	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create flow: %v", err), http.StatusInternalServerError)
		return
	}

	// Return the flow ID
	json.NewEncoder(w).Encode(map[string]string{"id": flowID})
}

// getFlowHandler gets a flow by ID
func (s *Server) getFlowHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Get flow ID from URL
	vars := mux.Vars(r)
	flowID := vars["id"]

	// Get flow
	flow, err := s.FlowManager.GetFlow(flowID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Flow not found: %v", err), http.StatusNotFound)
		return
	}

	// Return the flow
	json.NewEncoder(w).Encode(flow)
}

// executeCommandHandler executes a command in a flow
func (s *Server) executeCommandHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Get flow ID from URL
	vars := mux.Vars(r)
	flowID := vars["id"]

	// Parse request body
	var request CommandRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Get flow
	flow, err := s.FlowManager.GetFlow(flowID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Flow not found: %v", err), http.StatusNotFound)
		return
	}

	// Check if it's a planning flow - need to use type switch since we're working with an interface
	var commandID string
	var execErr error

	// We need to check if the flow supports command execution
	switch f := flow.(type) {
	case interface{ ExecuteCommandInBackground(string) (string, error) }:
		// Execute command in background using the planning flow
		commandID, execErr = f.ExecuteCommandInBackground(request.Command)
	default:
		http.Error(w, "Flow does not support command execution", http.StatusBadRequest)
		return
	}

	// Check for execution errors
	if execErr != nil {
		// System-level execution error
		http.Error(w, fmt.Sprintf("Failed to execute command: %v", execErr), http.StatusInternalServerError)
		return
	}

	// Return the command ID
	response := CommandResponse{
		Success:   true,
		CommandID: commandID,
	}

	json.NewEncoder(w).Encode(response)
}

// getCommandStatusHandler gets the status of a command
func (s *Server) getCommandStatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Get flow ID and command ID from URL
	vars := mux.Vars(r)
	flowID := vars["id"]
	commandID := vars["command_id"]

	// Get command status
	status, err := s.FlowManager.GetCommandStatus(flowID, commandID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get command status: %v", err), http.StatusInternalServerError)
		return
	}

	// Convert to response format
	response := CommandStatusResponse{
		Running:    status.Running,
		ExitCode:   status.ExitCode,
		Output:     status.Output,
		Error:      status.Error,
		OutputList: status.OutputList,
		ErrorList:  status.ErrorList,
		Duration:   status.Duration,
	}

	// Return success with status details even if command failed
	json.NewEncoder(w).Encode(response)
}

// streamFlowHandler streams flow updates
func (s *Server) streamFlowHandler(w http.ResponseWriter, r *http.Request) {
	// Get flow ID from URL
	vars := mux.Vars(r)
	flowID := vars["id"]

	// Upgrade HTTP connection to WebSocket
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}

	// Register client
	s.ClientsMutex.Lock()
	s.Clients[flowID] = append(s.Clients[flowID], conn)
	s.ClientsMutex.Unlock()

	// Clean up when the connection is closed
	defer func() {
		conn.Close()
		s.removeClient(flowID, conn)
	}()

	// Keep the connection alive
	for {
		// Read messages (just to detect disconnection)
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

// streamCommandHandler streams command updates
func (s *Server) streamCommandHandler(w http.ResponseWriter, r *http.Request) {
	// Get flow ID and command ID from URL
	vars := mux.Vars(r)
	flowID := vars["id"]
	commandID := vars["command_id"]

	// Upgrade HTTP connection to WebSocket
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}

	// Clean up when the connection is closed
	defer conn.Close()

	// Stream command status updates
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Create a ticker to poll for updates - use 1 second as mentioned in the memories
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// Keep track of the last status to detect changes
	var lastStatus *executor.BackgroundCommandStatus

	// Keep track of processed output and error lines to avoid duplicates
	processedOutputLines := 0
	processedErrorLines := 0

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Get command status
			status, err := s.FlowManager.GetCommandStatus(flowID, commandID)
			if err != nil {
				// Send error message and close connection
				conn.WriteJSON(map[string]string{"error": fmt.Sprintf("Failed to get command status: %v", err)})
				return
			}

			// Always send updates for streaming output
			// Check if there are new output or error lines
			hasNewOutput := lastStatus == nil ||
				len(status.OutputList) > processedOutputLines ||
				len(status.ErrorList) > processedErrorLines

			// Add running state and exit code changes if we have a previous status
			if lastStatus != nil {
				hasNewOutput = hasNewOutput || status.Running != lastStatus.Running || status.ExitCode != lastStatus.ExitCode
			}

			// Extract only new output lines
			newOutputList := []string{}
			newErrorList := []string{}

			if len(status.OutputList) > processedOutputLines {
				newOutputList = status.OutputList[processedOutputLines:]
				processedOutputLines = len(status.OutputList)
			}

			if len(status.ErrorList) > processedErrorLines {
				newErrorList = status.ErrorList[processedErrorLines:]
				processedErrorLines = len(status.ErrorList)
			}

			// Send update if there's new output or status change
			if hasNewOutput || lastStatus == nil || statusChanged(lastStatus, status) {
				// Send updated status with incremental output
				response := CommandStatusResponse{
					Running:    status.Running,
					ExitCode:   status.ExitCode,
					Output:     status.Output,
					Error:      status.Error,
					OutputList: newOutputList, // Only send new lines
					ErrorList:  newErrorList,  // Only send new lines
					Duration:   status.Duration,
					// Add flags to indicate if this is incremental or complete output
					Incremental: true,
				}

				if err := conn.WriteJSON(response); err != nil {
					log.Printf("Failed to write to WebSocket: %v", err)
					return
				}

				// Update last status
				lastStatus = status

				// If command is no longer running and we've sent all output, we can close the connection
				if !status.Running && len(newOutputList) == 0 && len(newErrorList) == 0 {
					// Send one final complete update with all output
					finalResponse := CommandStatusResponse{
						Running:     status.Running,
						ExitCode:    status.ExitCode,
						Output:      status.Output,
						Error:       status.Error,
						OutputList:  status.OutputList, // Send complete output
						ErrorList:   status.ErrorList,  // Send complete error
						Duration:    status.Duration,
						Incremental: false,
						Complete:    true,
					}

					if err := conn.WriteJSON(finalResponse); err != nil {
						log.Printf("Failed to write final status to WebSocket: %v", err)
					}

					// Close the connection gracefully
					conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "Command completed"))
					return
				}
			}
		}
	}
}

// removeClient removes a client from the clients map
func (s *Server) removeClient(flowID string, conn *websocket.Conn) {
	s.ClientsMutex.Lock()
	defer s.ClientsMutex.Unlock()

	clients := s.Clients[flowID]
	for i, client := range clients {
		if client == conn {
			s.Clients[flowID] = append(clients[:i], clients[i+1:]...)
			break
		}
	}
}

// BroadcastFlowUpdate broadcasts a flow update to all connected clients
func (s *Server) BroadcastFlowUpdate(flowID string, message interface{}) {
	s.ClientsMutex.Lock()
	clients := s.Clients[flowID]
	s.ClientsMutex.Unlock()

	for _, client := range clients {
		if err := client.WriteJSON(message); err != nil {
			log.Printf("Failed to write to WebSocket: %v", err)
			// Don't remove client here to avoid deadlock
		}
	}
}

// statusChanged checks if the command status has changed
func statusChanged(old, new *executor.BackgroundCommandStatus) bool {
	// Check if running state changed
	if old.Running != new.Running {
		return true
	}

	// Check if exit code changed
	if old.ExitCode != new.ExitCode {
		return true
	}

	// Check if output or error lines changed
	if len(old.OutputList) != len(new.OutputList) || len(old.ErrorList) != len(new.ErrorList) {
		return true
	}

	// Check if output content changed (for partial updates)
	if old.Output != new.Output || old.Error != new.Error {
		return true
	}

	// Check if duration changed significantly (more than 100ms)
	if new.Duration > old.Duration+0.1 {
		return true
	}

	// No significant changes
	return false
}
