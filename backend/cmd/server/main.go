/*
Package main provides a WebSocket-based configuration management server.

Key Features:
- Real-time configuration updates via WebSocket
- Multi-page support with individual configurations
- Theme and layout management
- Chat message handling
- CORS support
- Static file serving

API Endpoints:
- GET  /api/pages           - List all available pages
- GET  /api/pages/{pageId}  - Get configuration for specific page
- POST /api/pages/{pageId}  - Update configuration for specific page
- POST /api/reset           - Reset configuration to defaults
- GET  /ws                  - WebSocket endpoint for real-time updates

Usage:

	Start the server:
	$ go run main.go

	The server will start on port 8080 and handle both HTTP and WebSocket connections.

Security Notes:
  - This implementation allows all CORS origins
  - WebSocket connections accept all origins
  - No authentication is implemented
  - Intended for development/demo use only
*/

package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/rs/cors"
)

// SharedConfig represents the core configuration shared across a chat instance.
// It contains all necessary information for rendering the chat interface and managing messages.
type SharedConfig struct {
	DisplayMessage string    `json:"message"`     // Message to be displayed in the chat header
	CurrentColor   string    `json:"color"`       // Current theme color
	Theme          string    `json:"theme"`       // UI theme (light/dark)
	ChatPartner    ChatUser  `json:"chatPartner"` // Information about the chat partner
	Messages       []Message `json:"messages"`    // Array of chat messages
}

// ChatUser represents a user in the chat system with their basic information.
type ChatUser struct {
	Name   string `json:"name"`   // Display name of the user
	Status string `json:"status"` // Online status (Online/Offline/Away)
	Avatar string `json:"avatar"` // URL to user's avatar image
}

// Message represents a single chat message in the system.
type Message struct {
	ID        string    `json:"id"`        // Unique identifier for the message
	Content   string    `json:"content"`   // Message content
	Sender    string    `json:"sender"`    // Name of message sender
	Timestamp time.Time `json:"timestamp"` // Time when message was sent
}

// UIConfig defines the complete UI configuration structure sent to clients.
type UIConfig struct {
	Layout     string      `json:"layout"`     // Layout type (e.g., "default", "compact")
	Components []Component `json:"components"` // UI components to render
	Theme      ThemeConfig `json:"theme"`      // Theme configuration
	UpdatedAt  string      `json:"updatedAt"`  // Last update timestamp
}

// Component represents a single UI component in the interface.
type Component struct {
	Type       string            `json:"type"`       // Component type (e.g., "chat-header")
	ID         string            `json:"id"`         // Unique component identifier
	Content    string            `json:"content"`    // Component content
	Properties map[string]string `json:"properties"` // Additional component properties
}

// ThemeConfig defines the visual theme settings for the UI.
type ThemeConfig struct {
	PrimaryColor   string `json:"primaryColor"`   // Main theme color
	SecondaryColor string `json:"secondaryColor"` // Secondary theme color
	FontSize       string `json:"fontSize"`       // Base font size
}

// PageConfig stores configuration for individual chat pages.
type PageConfig struct {
	PageID      string       `json:"pageId"`      // Unique page identifier
	DisplayName string       `json:"displayName"` // Human-readable page name
	Config      SharedConfig `json:"config"`      // Page-specific configuration
}

// Client represents a connected WebSocket client.
type Client struct {
	conn   *websocket.Conn // WebSocket connection
	pageID string          // ID of the page client is subscribed to
}

// Global variables for managing state
var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins for demo purposes
		},
	}

	sharedConfig SharedConfig                  // Global shared configuration
	configLock   sync.RWMutex                  // Mutex for sharedConfig access
	clients      = make(map[*Client]bool)      // Connected clients
	clientsLock  sync.RWMutex                  // Mutex for clients map access
	pages        = make(map[string]PageConfig) // Page configurations
	pagesLock    sync.RWMutex                  // Mutex for pages map access
)

// broadcastToClients sends the updated UIConfig to all clients subscribed to a specific page.
// If pageID is empty, broadcasts to all clients.
func broadcastToClients(config UIConfig, pageID string) {
	clientsLock.Lock()
	defer clientsLock.Unlock()

	for client := range clients {
		if client.pageID == pageID {
			err := client.conn.WriteJSON(config)
			if err != nil {
				log.Printf("Websocket error: %v", err)
				client.conn.Close()
				delete(clients, client)
			}
		}
	}
}

// handleWebSocket manages WebSocket connections for real-time updates.
// It handles client connection, initial configuration sending, and cleanup on disconnect.
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	pageID := r.URL.Query().Get("pageId")
	log.Printf("WebSocket connection requested for pageId: %s", pageID)

	if pageID == "" {
		http.Error(w, "PageID is required", http.StatusBadRequest)
		return
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Websocket upgrade error: %v", err)
		return
	}

	client := &Client{
		conn:   ws,
		pageID: pageID,
	}

	clientsLock.Lock()
	clients[client] = true
	clientsLock.Unlock()

	// Send initial config for the requested page
	pagesLock.RLock()
	if page, exists := pages[pageID]; exists {
		log.Printf("Found page config for %s: %+v", pageID, page)
		// Create UI config directly from page config
		newConfig := UIConfig{
			Layout:    page.Config.Theme,
			UpdatedAt: time.Now().Format(time.RFC3339),
			Theme: ThemeConfig{
				PrimaryColor:   page.Config.CurrentColor,
				SecondaryColor: "#000000",
				FontSize:       "16px",
			},
			Components: []Component{
				{
					Type:    "chat-header",
					ID:      "chat-partner-info",
					Content: page.Config.DisplayMessage,
					Properties: map[string]string{
						"userName":   page.Config.ChatPartner.Name,
						"userStatus": page.Config.ChatPartner.Status,
					},
				},
				{
					Type:    "chat-messages",
					ID:      "message-list",
					Content: "",
					Properties: map[string]string{
						"messages": string(mustEncodeJSON(page.Config.Messages)),
					},
				},
			},
		}
		log.Printf("Sending initial config: %+v", newConfig)
		if err := ws.WriteJSON(newConfig); err != nil {
			log.Printf("Error sending initial config: %v", err)
		}
	}
	pagesLock.RUnlock()

	// Keep connection alive and clean up on disconnect
	for {
		_, _, err := ws.ReadMessage()
		if err != nil {
			clientsLock.Lock()
			delete(clients, client)
			clientsLock.Unlock()
			break
		}
	}
}

// buildUIConfig creates a new UIConfig instance based on provided parameters
// and current shared configuration state.
func buildUIConfig(message, color, theme string) UIConfig {
	configLock.RLock()
	defer configLock.RUnlock()

	return UIConfig{
		Layout:    theme,
		UpdatedAt: time.Now().Format(time.RFC3339),
		Theme: ThemeConfig{
			PrimaryColor:   color,
			SecondaryColor: "#000000",
			FontSize:       "16px",
		},
		Components: []Component{
			{
				Type:    "chat-header",
				ID:      "chat-partner-info",
				Content: message,
				Properties: map[string]string{
					"userName":   sharedConfig.ChatPartner.Name,
					"userStatus": sharedConfig.ChatPartner.Status,
				},
			},
			{
				Type:    "chat-messages",
				ID:      "message-list",
				Content: "",
				Properties: map[string]string{
					"messages": string(mustEncodeJSON(sharedConfig.Messages)),
				},
			},
		},
	}
}

// mustEncodeJSON marshals an interface to JSON bytes, returning an empty array on error.
// This is a helper function for safe JSON encoding.
func mustEncodeJSON(v interface{}) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		log.Printf("Error encoding JSON: %v", err)
		return []byte("[]")
	}
	return data
}

// updateUIConfig handles HTTP requests to update the shared configuration.
// It accepts partial updates and broadcasts changes to all connected clients.
func updateUIConfig(w http.ResponseWriter, r *http.Request) {
	var update SharedConfig
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	configLock.Lock()
	if update.DisplayMessage != "" {
		sharedConfig.DisplayMessage = update.DisplayMessage
	}
	if update.CurrentColor != "" {
		sharedConfig.CurrentColor = update.CurrentColor
	}
	if update.Theme != "" {
		sharedConfig.Theme = update.Theme
	}
	if update.ChatPartner.Name != "" {
		sharedConfig.ChatPartner = update.ChatPartner
	}
	if len(update.Messages) > 0 {
		sharedConfig.Messages = update.Messages
	}
	configLock.Unlock()

	// Build and broadcast new config
	newConfig := buildUIConfig(sharedConfig.DisplayMessage, sharedConfig.CurrentColor, sharedConfig.Theme)
	broadcastToClients(newConfig, "")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
}

// resetUIConfig resets the shared configuration to default values
// and broadcasts the reset to all connected clients.
func resetUIConfig(w http.ResponseWriter, r *http.Request) {
	configLock.Lock()
	sharedConfig = SharedConfig{
		DisplayMessage: "Welcome to Chat",
		CurrentColor:   "#ffffff",
		Theme:          "light",
		ChatPartner: ChatUser{
			Name:   "Chat Partner",
			Status: "Offline",
			Avatar: "",
		},
		Messages: []Message{},
	}
	configLock.Unlock()

	// Build and broadcast new config
	newConfig := buildUIConfig(sharedConfig.DisplayMessage, sharedConfig.CurrentColor, sharedConfig.Theme)
	broadcastToClients(newConfig, "")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "reset"})
}

// updatePageConfig handles updates to page-specific configurations.
// It validates the update, stores it, and broadcasts changes to relevant clients.
func updatePageConfig(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	pageID := vars["pageId"]

	var update PageConfig
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if pageID == "" {
		http.Error(w, "PageID is required", http.StatusBadRequest)
		return
	}

	pagesLock.Lock()
	// Ensure all required fields are set
	if update.Config.DisplayMessage == "" {
		update.Config.DisplayMessage = "Welcome to Chat"
	}
	if update.Config.CurrentColor == "" {
		update.Config.CurrentColor = "#ffffff"
	}
	if update.Config.Theme == "" {
		update.Config.Theme = "light"
	}
	pages[pageID] = update
	pagesLock.Unlock()

	// Build and broadcast new config for this page
	newConfig := UIConfig{
		Layout:    update.Config.Theme,
		UpdatedAt: time.Now().Format(time.RFC3339),
		Theme: ThemeConfig{
			PrimaryColor:   update.Config.CurrentColor,
			SecondaryColor: "#000000",
			FontSize:       "16px",
		},
		Components: []Component{
			{
				Type:    "chat-header",
				ID:      "chat-partner-info",
				Content: update.Config.DisplayMessage,
				Properties: map[string]string{
					"userName":   update.Config.ChatPartner.Name,
					"userStatus": update.Config.ChatPartner.Status,
				},
			},
			{
				Type:    "chat-messages",
				ID:      "message-list",
				Content: "",
				Properties: map[string]string{
					"messages": string(mustEncodeJSON(update.Config.Messages)),
				},
			},
		},
	}

	broadcastToClients(newConfig, pageID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "updated",
		"pageId": pageID,
	})
}

// listPages returns a JSON array of all available pages and their display names.
func listPages(w http.ResponseWriter, r *http.Request) {
	pagesLock.RLock()
	pageList := make([]map[string]string, 0)
	for _, page := range pages {
		pageList = append(pageList, map[string]string{
			"pageId":      page.PageID,
			"displayName": page.DisplayName,
		})
	}
	pagesLock.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pageList)
}

// getPageConfig retrieves and returns the configuration for a specific page.
// Returns 404 if the page doesn't exist.
func getPageConfig(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	pageID := vars["pageId"]

	pagesLock.RLock()
	page, exists := pages[pageID]
	pagesLock.RUnlock()

	if !exists {
		http.Error(w, "Page not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(page)
}

// main initializes and starts the HTTP server with WebSocket support.
// It sets up routes, middleware, and begins listening for connections.
func main() {
	r := mux.NewRouter()

	// CORS middleware first
	c := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type", "Authorization"},
	})

	// Debug middleware
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Printf("REQUEST: %s %s", r.Method, r.URL.Path)
			next.ServeHTTP(w, r)
		})
	})

	// API subrouter
	api := r.PathPrefix("/api").Subrouter()
	api.HandleFunc("/pages", listPages).Methods("GET")
	api.HandleFunc("/pages/{pageId}", updatePageConfig).Methods("POST")
	api.HandleFunc("/pages/{pageId}", getPageConfig).Methods("GET")
	api.HandleFunc("/reset", resetUIConfig).Methods("POST")

	// WebSocket route
	r.HandleFunc("/ws", handleWebSocket)

	// Static files last
	fs := http.FileServer(http.Dir("frontend/dist"))
	r.PathPrefix("/").Handler(fs)

	// Handle with CORS
	handler := c.Handler(r)

	log.Println("Server starting on :8080...")
	log.Printf("Routes registered:")
	log.Printf("- GET /api/pages")
	log.Printf("- POST /api/pages/{pageId}")
	log.Printf("- GET /api/pages/{pageId}")
	log.Printf("- POST /api/reset")
	log.Printf("- GET /ws")

	log.Fatal(http.ListenAndServe(":8080", handler))
}
