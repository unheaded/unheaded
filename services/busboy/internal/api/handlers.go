package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"unheaded/services/busboy/internal/busboy"
	"unheaded/services/busboy/internal/member"
	"unheaded/services/busboy/internal/room"
)

// maxRequestBodySize is the maximum allowed request body size (1MB).
const maxRequestBodySize = 1 * 1024 * 1024

// Server holds the API dependencies
type Server struct {
	RoomManager            *room.Manager
	MemberManager          *member.Manager
	Busboy                 *busboy.Busboy
	PendingApprovalTimeout time.Duration
	AdminAPIKey            string // API key for admin endpoints
}

// NewServer creates a new API server
func NewServer(roomMgr *room.Manager, memberMgr *member.Manager, msgBusboy *busboy.Busboy, pendingApprovalTimeout time.Duration) *Server {
	return &Server{
		RoomManager:            roomMgr,
		MemberManager:          memberMgr,
		Busboy:                 msgBusboy,
		PendingApprovalTimeout: pendingApprovalTimeout,
		AdminAPIKey:            "", // Must be set via SetAdminAPIKey
	}
}

// SetAdminAPIKey sets the admin API key for protected endpoints
func (s *Server) SetAdminAPIKey(key string) {
	s.AdminAPIKey = key
}

type JoinRoomRequest struct {
	RoomID    string `json:"room_id"`
	Name      string `json:"name"`
	PublicKey string `json:"public_key,omitempty"`
}

type JoinRoomResponse struct {
	MemberID uuid.UUID     `json:"member_id"`
	Status   member.Status `json:"status"`
	Message  string        `json:"message"`
}

type SendMessageRequest struct {
	MemberID uuid.UUID `json:"member_id"`
	RoomID   string    `json:"room_id"`
	Content  string    `json:"content"`
}

type SendMessageResponse struct {
	MessageID   uuid.UUID `json:"message_id"`
	Timestamp   time.Time `json:"timestamp"`
	Overwritten bool      `json:"overwritten"`
}

type DeleteMessageRequest struct {
	MemberID  uuid.UUID `json:"member_id"`
	MessageID uuid.UUID `json:"message_id"`
	RoomID    string    `json:"room_id"`
	Content   string    `json:"content"`
}

type DeleteMessageResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type GetMessagesResponse struct {
	Messages []MessageResponse `json:"messages"`
	Count    int               `json:"count"`
}

type MessageResponse struct {
	ID        uuid.UUID `json:"id"`
	CreatorID uuid.UUID `json:"creator_id"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Handlers

// JoinRoom handles member join requests (creates pending member)
func (s *Server) JoinRoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req JoinRoomRequest
	// SECURITY: Enforce body size limit to prevent denial-of-service
	if err := json.NewDecoder(io.LimitReader(r.Body, maxRequestBodySize)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.RoomID == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "room_id and name are required")
		return
	}

	// Create or get room
	s.RoomManager.GetOrCreate(req.RoomID, req.RoomID)

	// Create pending member with timeout
	newMember := s.MemberManager.RequestJoin(req.RoomID, req.Name, req.PublicKey, s.PendingApprovalTimeout)

	resp := JoinRoomResponse{
		MemberID: newMember.ID,
		Status:   newMember.Status,
		Message:  "join request submitted, awaiting admin approval",
	}

	writeJSON(w, http.StatusCreated, resp)
}

// SendMessage handles sending a message to a room
func (s *Server) SendMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req SendMessageRequest
	// SECURITY: Enforce body size limit to prevent denial-of-service
	if err := json.NewDecoder(io.LimitReader(r.Body, maxRequestBodySize)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Content == "" || req.RoomID == "" {
		writeError(w, http.StatusBadRequest, "content and room_id are required")
		return
	}

	// Check if member is approved
	if !s.MemberManager.IsApproved(req.MemberID) {
		writeError(w, http.StatusForbidden, "member not approved or not found")
		return
	}

	// Get room
	rm, err := s.RoomManager.Get(req.RoomID)
	if err != nil {
		writeError(w, http.StatusNotFound, "room not found")
		return
	}

	// Send message
	msg, overwritten := rm.SendMessage(req.MemberID, req.Content)

	// Broadcast to subscribers
	if s.Busboy != nil {
		s.Busboy.PublishMessageCreated(msg)
	}

	resp := SendMessageResponse{
		MessageID:   msg.ID,
		Timestamp:   msg.Timestamp,
		Overwritten: overwritten,
	}

	writeJSON(w, http.StatusCreated, resp)
}

// DeleteMessage handles message deletion (with triple verification)
func (s *Server) DeleteMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req DeleteMessageRequest
	// SECURITY: Enforce body size limit to prevent denial-of-service
	if err := json.NewDecoder(io.LimitReader(r.Body, maxRequestBodySize)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Content == "" || req.RoomID == "" {
		writeError(w, http.StatusBadRequest, "message_id, content, and room_id are required")
		return
	}

	// Check if member is approved
	if !s.MemberManager.IsApproved(req.MemberID) {
		writeError(w, http.StatusForbidden, "member not approved or not found")
		return
	}

	// Get room
	rm, err := s.RoomManager.Get(req.RoomID)
	if err != nil {
		writeError(w, http.StatusNotFound, "room not found")
		return
	}

	// Delete message (triple verification: creator + content + UUID)
	success := rm.DeleteMessage(req.MessageID, req.MemberID, req.Content)

	if !success {
		resp := DeleteMessageResponse{
			Success: false,
			Message: "message not found or verification failed",
		}
		writeJSON(w, http.StatusNotFound, resp)
		return
	}

	// Broadcast deletion to subscribers
	if s.Busboy != nil {
		s.Busboy.PublishMessageDeleted(req.RoomID, req.MessageID)
	}

	resp := DeleteMessageResponse{
		Success: true,
		Message: "message deleted successfully",
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetMessages retrieves messages from a room
func (s *Server) GetMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	roomID := r.URL.Query().Get("room_id")
	memberIDStr := r.URL.Query().Get("member_id")

	if roomID == "" || memberIDStr == "" {
		writeError(w, http.StatusBadRequest, "room_id and member_id are required")
		return
	}

	memberID, err := uuid.Parse(memberIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid member_id")
		return
	}

	// Check if member is approved
	if !s.MemberManager.IsApproved(memberID) {
		writeError(w, http.StatusForbidden, "member not approved or not found")
		return
	}

	// Get room
	rm, err := s.RoomManager.Get(roomID)
	if err != nil {
		writeError(w, http.StatusNotFound, "room not found")
		return
	}

	// Get messages
	messages := rm.GetMessages()

	resp := GetMessagesResponse{
		Messages: make([]MessageResponse, len(messages)),
		Count:    len(messages),
	}

	for i, msg := range messages {
		resp.Messages[i] = MessageResponse{
			ID:        msg.ID,
			CreatorID: msg.CreatorID,
			Content:   msg.Content,
			Timestamp: msg.Timestamp,
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetPendingMembers retrieves all pending member requests (admin only)
func (s *Server) GetPendingMembers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Admin authentication
	if err := s.requireAdminAuth(r); err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	pending := s.MemberManager.GetAllPending()
	writeJSON(w, http.StatusOK, pending)
}

// ApproveMember approves a pending member (admin only)
func (s *Server) ApproveMember(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Admin authentication
	if err := s.requireAdminAuth(r); err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	var req struct {
		MemberID   string `json:"member_id"`
		ApprovedBy string `json:"approved_by"`
	}

	// SECURITY: Enforce body size limit to prevent denial-of-service
	if err := json.NewDecoder(io.LimitReader(r.Body, maxRequestBodySize)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	memberID, err := uuid.Parse(req.MemberID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid member_id")
		return
	}

	if err := s.MemberManager.Approve(memberID, req.ApprovedBy); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "approved"})
}

// DenyMember denies a pending member (admin only)
func (s *Server) DenyMember(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Admin authentication
	if err := s.requireAdminAuth(r); err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	var req struct {
		MemberID string `json:"member_id"`
	}

	// SECURITY: Enforce body size limit to prevent denial-of-service
	if err := json.NewDecoder(io.LimitReader(r.Body, maxRequestBodySize)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	memberID, err := uuid.Parse(req.MemberID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid member_id")
		return
	}

	if err := s.MemberManager.Deny(memberID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "denied"})
}

// ApproveAllMembers approves all pending members (admin only)
func (s *Server) ApproveAllMembers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Admin authentication
	if err := s.requireAdminAuth(r); err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	var req struct {
		ApprovedBy string `json:"approved_by"`
	}

	// Body is optional, default to "admin" if not provided
	approvedBy := "admin"
	if r.Body != nil {
		// SECURITY: Enforce body size limit to prevent denial-of-service
		_ = json.NewDecoder(io.LimitReader(r.Body, maxRequestBodySize)).Decode(&req)
		if req.ApprovedBy != "" {
			approvedBy = req.ApprovedBy
		}
	}

	count := s.MemberManager.ApproveAll(approvedBy)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":         "approved",
		"approved_count": count,
		"message":        fmt.Sprintf("approved %d pending member(s)", count),
	})
}

// Health check endpoint
func (s *Server) Health(w http.ResponseWriter, r *http.Request) {
	health := map[string]interface{}{
		"status":        "ok",
		"timestamp":     time.Now(),
		"rooms":         s.RoomManager.Count(),
		"total_members": s.MemberManager.Count(),
	}
	writeJSON(w, http.StatusOK, health)
}

// Helper functions

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Error().Err(err).Msg("failed to encode JSON response")
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	resp := ErrorResponse{
		Error:   http.StatusText(status),
		Code:    status,
		Message: message,
	}
	writeJSON(w, status, resp)
}

// requireAdminAuth validates admin authentication for protected endpoints
// Returns nil if authentication succeeds, error otherwise
func (s *Server) requireAdminAuth(r *http.Request) error {
	// Check if admin API key is configured
	if s.AdminAPIKey == "" {
		log.Warn().Msg("admin_auth_not_configured")
		return fmt.Errorf("admin authentication not configured")
	}

	// Check X-Admin-API-Key header
	providedKey := r.Header.Get("X-Admin-API-Key")

	// Also check Authorization header with Bearer token
	if providedKey == "" {
		authHeader := r.Header.Get("Authorization")
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			providedKey = authHeader[7:]
		}
	}

	// Validate presence
	if providedKey == "" {
		log.Warn().
			Str("path", r.URL.Path).
			Str("remote_addr", r.RemoteAddr).
			Msg("admin_auth_missing_key")
		return fmt.Errorf("admin API key required")
	}

	// Constant-time comparison to prevent timing attacks
	if !secureCompare(providedKey, s.AdminAPIKey) {
		log.Warn().
			Str("path", r.URL.Path).
			Str("remote_addr", r.RemoteAddr).
			Msg("admin_auth_invalid_key")
		return fmt.Errorf("invalid admin API key")
	}

	log.Info().
		Str("path", r.URL.Path).
		Str("remote_addr", r.RemoteAddr).
		Msg("admin_auth_success")

	return nil
}

// secureCompare performs constant-time string comparison to prevent timing attacks
func secureCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var result byte
	for i := 0; i < len(a); i++ {
		result |= a[i] ^ b[i]
	}
	return result == 0
}
