package httpserver

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

const defaultSessionCookieName = "hunch_session"

type accountContextKey struct{}

type hunchAPI struct {
	sessionCookieName string
	store             *hunchStore
	now               func() time.Time
}

type hunchStore struct {
	mu       sync.Mutex
	accounts map[string]*hunchAccount
}

type hunchAccount struct {
	AccountID            string
	UserID               string
	Profile              profileState
	SetupSessions        map[string]*setupSessionState
	ActiveSetupSessionID string
	SelfModelSnapshotID  string
	Claims               map[string]*selfModelClaim
	Conversations        map[string]*conversationState
	Feedback             map[string]*conversationFeedback
}

type profileState struct {
	DisplayName       *string
	Locale            string
	Timezone          string
	SetupStatus       string
	SetupCompletedAt  *time.Time
	ConversationStyle conversationStyle
	FocusAreas        []string
	Boundaries        profileBoundaries
	ReviewBeforeUse   bool
	Version           int
	UpdatedAt         time.Time
}

type conversationStyle struct {
	Depth            string `json:"depth"`
	Directness       string `json:"directness"`
	DefaultLanguage  string `json:"default_language"`
	ReflectionFormat string `json:"reflection_format"`
}

type profileBoundaries struct {
	AvoidTopics       []string `json:"avoid_topics"`
	SensitiveTopicsOK []string `json:"sensitive_topics_ok"`
	MemoryOptIn       bool     `json:"memory_opt_in"`
}

type setupSessionState struct {
	ID              string
	Status          string
	ClientContext   clientContext
	Answers         map[string]*setupAnswer
	CreatedAt       time.Time
	UpdatedAt       time.Time
	CompletedAt     *time.Time
	MemoryOptIn     bool
	ReviewBeforeUse bool
}

type clientContext struct {
	Locale     string `json:"locale"`
	Timezone   string `json:"timezone"`
	Platform   string `json:"platform"`
	AppVersion string `json:"app_version"`
}

type setupAnswer struct {
	ID         string         `json:"answer_id"`
	StepID     string         `json:"step_id"`
	Answer     map[string]any `json:"answer"`
	AnsweredAt time.Time      `json:"answered_at"`
}

type setupStep struct {
	StepID      string         `json:"step_id"`
	Type        string         `json:"type"`
	Prompt      string         `json:"prompt"`
	Required    bool           `json:"required"`
	Options     []setupOption  `json:"options,omitempty"`
	Constraints map[string]any `json:"constraints,omitempty"`
}

type setupOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type selfModelClaim struct {
	ID          string
	Field       string
	Label       string
	Source      string
	EvidenceIDs []string
	SourceTypes []string
	Confidence  float64
	FirstSeen   string
	LastSeen    string
	Stability   string
	Status      string
	ReviewState string
	UpdatedAt   time.Time
}

type conversationState struct {
	ID            string
	Mode          string
	Title         string
	Status        string
	Messages      []*conversationMessage
	CreatedAt     time.Time
	LastMessageAt time.Time
}

type conversationMessage struct {
	ID        string
	Role      string
	Content   map[string]any
	Metadata  map[string]any
	CreatedAt time.Time
}

type conversationFeedback struct {
	ID             string
	ConversationID string
	MessageID      string
	Rating         string
	Tags           []string
	Comment        string
	CreatedAt      time.Time
}

var setupSteps = []setupStep{
	{
		StepID:   "display_name",
		Type:     "short_text",
		Prompt:   "What should Hunch call you?",
		Required: true,
		Constraints: map[string]any{
			"min_length": 1,
			"max_length": 40,
		},
	},
	{
		StepID:   "focus_areas",
		Type:     "multi_select",
		Prompt:   "What would you most like Hunch to help with right now?",
		Required: true,
		Options: []setupOption{
			{Value: "career", Label: "Work and career"},
			{Value: "relationships", Label: "Relationships"},
			{Value: "self_understanding", Label: "Self-understanding"},
			{Value: "faith", Label: "Faith and values"},
		},
		Constraints: map[string]any{
			"min_items": 1,
			"max_items": 4,
		},
	},
	{
		StepID:   "current_context",
		Type:     "long_text",
		Prompt:   "What is one situation or question you want to make sense of?",
		Required: true,
		Constraints: map[string]any{
			"max_length": 2000,
		},
	},
	{
		StepID:   "conversation_style",
		Type:     "choice_group",
		Prompt:   "Which conversation style would feel most useful?",
		Required: true,
		Options: []setupOption{
			{Value: "gentle_structured", Label: "Calm and structured"},
			{Value: "direct_practical", Label: "Clear and practical"},
			{Value: "deep_reflective", Label: "Deep and reflective"},
		},
	},
	{
		StepID:   "values_and_boundaries",
		Type:     "values_and_boundaries",
		Prompt:   "Which values or boundaries should Hunch keep in view?",
		Required: true,
		Constraints: map[string]any{
			"max_items": 8,
		},
	},
	{
		StepID:   "memory_consent",
		Type:     "consent",
		Prompt:   "May Hunch use this setup and future conversations to update memory?",
		Required: true,
	},
}

func newHunchAPI(deps Dependencies) *hunchAPI {
	sessionCookieName := deps.SessionCookieName
	if sessionCookieName == "" {
		sessionCookieName = defaultSessionCookieName
	}

	return &hunchAPI{
		sessionCookieName: sessionCookieName,
		store: &hunchStore{
			accounts: make(map[string]*hunchAccount),
		},
		now: func() time.Time {
			return time.Now().UTC().Truncate(time.Second)
		},
	}
}

func (api *hunchAPI) Mount(router chi.Router) {
	router.Route("/v1", func(r chi.Router) {
		r.Use(api.requireSession)

		r.Get("/me/bootstrap", api.handleBootstrap)
		r.Post("/me/setup/sessions", api.handleCreateSetupSession)
		r.Get("/me/setup/sessions/{setup_session_id}", api.handleGetSetupSession)
		r.Post("/me/setup/sessions/{setup_session_id}/answers", api.handleSaveSetupAnswer)
		r.Post("/me/setup/sessions/{setup_session_id}/skip", api.handleSkipSetup)
		r.Post("/me/setup/sessions/{setup_session_id}/complete", api.handleCompleteSetup)

		r.Get("/me/profile", api.handleGetProfile)
		r.Patch("/me/profile", api.handlePatchProfile)
		r.Get("/me/self-model", api.handleGetSelfModel)
		r.Patch("/me/self-model/claims/{claim_id}", api.handlePatchClaim)

		r.Post("/conversations", api.handleCreateConversation)
		r.Get("/conversations", api.handleListConversations)
		r.Get("/conversations/{conversation_id}", api.handleGetConversation)
		r.Post("/conversations/{conversation_id}/messages", api.handleSendConversationMessage)
		r.Post("/conversations/{conversation_id}/messages/stream", api.handleStreamConversationMessage)
		r.Post("/conversations/{conversation_id}/feedback", api.handleConversationFeedback)
	})
}

func (api *hunchAPI) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(api.sessionCookieName)
		if err != nil || strings.TrimSpace(cookie.Value) == "" {
			writeAPIError(w, r, http.StatusUnauthorized, "unauthenticated", "missing or invalid session", nil)
			return
		}

		api.store.mu.Lock()
		account := api.store.accountForSession(cookie.Value, api.now())
		api.store.mu.Unlock()

		ctx := context.WithValue(r.Context(), accountContextKey{}, account)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *hunchStore) accountForSession(sessionToken string, now time.Time) *hunchAccount {
	hash := sha256.Sum256([]byte(sessionToken))
	key := hex.EncodeToString(hash[:])

	account, ok := s.accounts[key]
	if ok {
		return account
	}

	account = &hunchAccount{
		AccountID: "acc_" + newID(),
		UserID:    "usr_" + newID(),
		Profile: profileState{
			Locale:      "en-US",
			Timezone:    "UTC",
			SetupStatus: "not_started",
			ConversationStyle: conversationStyle{
				Depth:            "balanced",
				Directness:       "gentle",
				DefaultLanguage:  "en",
				ReflectionFormat: "structured",
			},
			Boundaries: profileBoundaries{
				AvoidTopics:       []string{},
				SensitiveTopicsOK: []string{},
				MemoryOptIn:       false,
			},
			Version:   1,
			UpdatedAt: now,
		},
		SetupSessions: make(map[string]*setupSessionState),
		Claims:        make(map[string]*selfModelClaim),
		Conversations: make(map[string]*conversationState),
		Feedback:      make(map[string]*conversationFeedback),
	}
	s.accounts[key] = account
	return account
}

func accountFromRequest(r *http.Request) *hunchAccount {
	account, _ := r.Context().Value(accountContextKey{}).(*hunchAccount)
	return account
}

func (api *hunchAPI) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	api.store.mu.Lock()
	defer api.store.mu.Unlock()

	account := accountFromRequest(r)
	writeJSON(w, http.StatusOK, api.bootstrapResponse(account))
}

func (api *hunchAPI) handleCreateSetupSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClientContext clientContext `json:"client_context"`
	}
	if !decodeRequestJSON(w, r, &req) {
		return
	}

	api.store.mu.Lock()
	defer api.store.mu.Unlock()

	account := accountFromRequest(r)
	if account.ActiveSetupSessionID != "" {
		if session := account.SetupSessions[account.ActiveSetupSessionID]; session != nil && session.Status == "in_progress" {
			writeJSON(w, http.StatusOK, setupSessionResponse(session))
			return
		}
	}

	now := api.now()
	session := &setupSessionState{
		ID:            "set_" + newID(),
		Status:        "in_progress",
		ClientContext: req.ClientContext,
		Answers:       make(map[string]*setupAnswer),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	account.SetupSessions[session.ID] = session
	account.ActiveSetupSessionID = session.ID

	if req.ClientContext.Locale != "" {
		account.Profile.Locale = req.ClientContext.Locale
	}
	if req.ClientContext.Timezone != "" {
		account.Profile.Timezone = req.ClientContext.Timezone
	}
	account.Profile.UpdatedAt = now

	writeJSON(w, http.StatusCreated, setupSessionResponse(session))
}

func (api *hunchAPI) handleGetSetupSession(w http.ResponseWriter, r *http.Request) {
	api.store.mu.Lock()
	defer api.store.mu.Unlock()

	account := accountFromRequest(r)
	session, ok := account.SetupSessions[chi.URLParam(r, "setup_session_id")]
	if !ok {
		writeAPIError(w, r, http.StatusNotFound, "not_found", "setup session not found", nil)
		return
	}

	writeJSON(w, http.StatusOK, setupSessionResponse(session))
}

func (api *hunchAPI) handleSaveSetupAnswer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StepID string         `json:"step_id"`
		Answer map[string]any `json:"answer"`
	}
	if !decodeRequestJSON(w, r, &req) {
		return
	}

	api.store.mu.Lock()
	defer api.store.mu.Unlock()

	account := accountFromRequest(r)
	session, ok := account.SetupSessions[chi.URLParam(r, "setup_session_id")]
	if !ok {
		writeAPIError(w, r, http.StatusNotFound, "not_found", "setup session not found", nil)
		return
	}
	if session.Status != "in_progress" {
		writeAPIError(w, r, http.StatusConflict, "conflict", "setup session is not in progress", nil)
		return
	}
	if err := validateSetupAnswer(session, req.StepID, req.Answer); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_request", err.Error(), map[string]any{"field": req.StepID})
		return
	}

	now := api.now()
	answerID := "ans_" + newID()
	if existing := session.Answers[req.StepID]; existing != nil {
		answerID = existing.ID
	}
	session.Answers[req.StepID] = &setupAnswer{
		ID:         answerID,
		StepID:     req.StepID,
		Answer:     cloneObject(req.Answer),
		AnsweredAt: now,
	}
	session.UpdatedAt = now

	writeJSON(w, http.StatusOK, map[string]any{
		"setup_session_id": session.ID,
		"saved_answer": map[string]any{
			"step_id":     req.StepID,
			"answer_id":   answerID,
			"answered_at": isoTime(now),
		},
		"current_step": currentSetupStep(session),
		"progress":     setupProgress(session),
	})
}

func (api *hunchAPI) handleSkipSetup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Reason string `json:"reason"`
	}
	if !decodeRequestJSON(w, r, &req) {
		return
	}

	api.store.mu.Lock()
	defer api.store.mu.Unlock()

	account := accountFromRequest(r)
	session, ok := account.SetupSessions[chi.URLParam(r, "setup_session_id")]
	if !ok {
		writeAPIError(w, r, http.StatusNotFound, "not_found", "setup session not found", nil)
		return
	}

	now := api.now()
	session.Status = "skipped"
	session.UpdatedAt = now
	account.ActiveSetupSessionID = ""
	account.Profile.SetupStatus = "skipped"
	account.Profile.SetupCompletedAt = nil
	account.Profile.UpdatedAt = now
	account.Profile.Version++

	writeJSON(w, http.StatusOK, map[string]any{
		"setup_session_id": session.ID,
		"status":           session.Status,
		"profile": map[string]any{
			"setup_status":       account.Profile.SetupStatus,
			"setup_completed_at": nil,
		},
		"next_action": map[string]any{
			"type":           "start_conversation",
			"suggested_mode": "light_reflection",
		},
	})
}

func (api *hunchAPI) handleCompleteSetup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MemoryOptIn     bool `json:"memory_opt_in"`
		ReviewBeforeUse bool `json:"review_before_use"`
	}
	if !decodeRequestJSON(w, r, &req) {
		return
	}

	api.store.mu.Lock()
	defer api.store.mu.Unlock()

	account := accountFromRequest(r)
	session, ok := account.SetupSessions[chi.URLParam(r, "setup_session_id")]
	if !ok {
		writeAPIError(w, r, http.StatusNotFound, "not_found", "setup session not found", nil)
		return
	}
	if session.Status == "completed" {
		writeJSON(w, http.StatusOK, completeSetupResponse(account, session))
		return
	}
	if session.Status != "in_progress" {
		writeAPIError(w, r, http.StatusConflict, "conflict", "setup session is not in progress", nil)
		return
	}
	if missing := missingRequiredSetupSteps(session); len(missing) > 0 {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_request", "required setup answers are missing", map[string]any{"missing_steps": missing})
		return
	}

	now := api.now()
	session.Status = "completed"
	session.CompletedAt = &now
	session.UpdatedAt = now
	session.MemoryOptIn = req.MemoryOptIn
	session.ReviewBeforeUse = req.ReviewBeforeUse
	account.ActiveSetupSessionID = ""

	applySetupAnswersToProfile(account, session, now)
	account.Profile.Boundaries.MemoryOptIn = req.MemoryOptIn
	account.Profile.ReviewBeforeUse = req.ReviewBeforeUse
	if req.MemoryOptIn {
		seedSelfModelClaims(account, session, now)
	}

	writeJSON(w, http.StatusOK, completeSetupResponse(account, session))
}

func (api *hunchAPI) handleGetProfile(w http.ResponseWriter, r *http.Request) {
	api.store.mu.Lock()
	defer api.store.mu.Unlock()

	account := accountFromRequest(r)
	writeJSON(w, http.StatusOK, profileResponse(account))
}

func (api *hunchAPI) handlePatchProfile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DisplayName       *string            `json:"display_name"`
		Locale            *string            `json:"locale"`
		Timezone          *string            `json:"timezone"`
		ConversationStyle *conversationStyle `json:"conversation_style"`
		FocusAreas        []string           `json:"focus_areas"`
		Boundaries        *profileBoundaries `json:"boundaries"`
		Version           *int               `json:"version"`
	}
	if !decodeRequestJSON(w, r, &req) {
		return
	}
	if req.Version == nil {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_request", "version is required", map[string]any{"field": "version"})
		return
	}

	api.store.mu.Lock()
	defer api.store.mu.Unlock()

	account := accountFromRequest(r)
	if *req.Version != account.Profile.Version {
		writeAPIError(w, r, http.StatusConflict, "conflict", "profile version is stale", map[string]any{"field": "version"})
		return
	}

	now := api.now()
	if req.DisplayName != nil {
		name := strings.TrimSpace(*req.DisplayName)
		if len(name) < 1 || len(name) > 40 {
			writeAPIError(w, r, http.StatusBadRequest, "invalid_request", "display name must be 1-40 characters", map[string]any{"field": "display_name"})
			return
		}
		account.Profile.DisplayName = &name
	}
	if req.Locale != nil && strings.TrimSpace(*req.Locale) != "" {
		account.Profile.Locale = strings.TrimSpace(*req.Locale)
	}
	if req.Timezone != nil && strings.TrimSpace(*req.Timezone) != "" {
		account.Profile.Timezone = strings.TrimSpace(*req.Timezone)
	}
	if req.ConversationStyle != nil {
		mergeConversationStyle(&account.Profile.ConversationStyle, *req.ConversationStyle)
	}
	if req.FocusAreas != nil {
		account.Profile.FocusAreas = cloneStrings(req.FocusAreas)
	}
	if req.Boundaries != nil {
		account.Profile.Boundaries = profileBoundaries{
			AvoidTopics:       cloneStrings(req.Boundaries.AvoidTopics),
			SensitiveTopicsOK: cloneStrings(req.Boundaries.SensitiveTopicsOK),
			MemoryOptIn:       req.Boundaries.MemoryOptIn,
		}
	}
	account.Profile.Version++
	account.Profile.UpdatedAt = now

	writeJSON(w, http.StatusOK, profileResponse(account))
}

func (api *hunchAPI) handleGetSelfModel(w http.ResponseWriter, r *http.Request) {
	api.store.mu.Lock()
	defer api.store.mu.Unlock()

	account := accountFromRequest(r)
	includeHidden := r.URL.Query().Get("include_hidden") == "true"
	fieldFilter := splitFilter(r.URL.Query().Get("fields"))
	reviewFilter := splitFilter(r.URL.Query().Get("review_state"))

	claims := make([]map[string]any, 0, len(account.Claims))
	for _, claim := range sortedClaims(account.Claims) {
		if !includeHidden && claim.ReviewState == "hidden" {
			continue
		}
		if len(fieldFilter) > 0 && !fieldFilter[claim.Field] {
			continue
		}
		if len(reviewFilter) > 0 && !reviewFilter[claim.ReviewState] {
			continue
		}
		claims = append(claims, selfModelClaimResponse(claim))
	}

	openQuestions := make([]map[string]any, 0)
	for _, claim := range sortedClaims(account.Claims) {
		if claim.Field != "open_questions" || claim.Status != "active" {
			continue
		}
		openQuestions = append(openQuestions, map[string]any{
			"claim_id":   claim.ID,
			"label":      claim.Label,
			"confidence": claim.Confidence,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"snapshot_id":    nullableString(account.SelfModelSnapshotID),
		"user_id":        account.UserID,
		"snapshot_at":    latestClaimUpdate(account.Claims),
		"claims":         claims,
		"open_questions": openQuestions,
	})
}

func (api *hunchAPI) handlePatchClaim(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action         string `json:"action"`
		CorrectedLabel string `json:"corrected_label"`
		Note           string `json:"note"`
	}
	if !decodeRequestJSON(w, r, &req) {
		return
	}

	api.store.mu.Lock()
	defer api.store.mu.Unlock()

	account := accountFromRequest(r)
	claim, ok := account.Claims[chi.URLParam(r, "claim_id")]
	if !ok {
		writeAPIError(w, r, http.StatusNotFound, "not_found", "self model claim not found", nil)
		return
	}

	now := api.now()
	switch req.Action {
	case "confirm":
		claim.ReviewState = "confirmed"
		claim.Status = "active"
	case "correct":
		label := strings.TrimSpace(req.CorrectedLabel)
		if label == "" || len(label) > 240 {
			writeAPIError(w, r, http.StatusBadRequest, "invalid_request", "corrected_label is required", map[string]any{"field": "corrected_label"})
			return
		}
		claim.Label = label
		claim.ReviewState = "corrected"
		claim.Status = "active"
		if claim.Confidence > 0.78 {
			claim.Confidence = 0.78
		}
	case "reject":
		claim.ReviewState = "rejected"
		claim.Status = "retired"
	case "hide":
		claim.ReviewState = "hidden"
		claim.Status = "active"
	default:
		writeAPIError(w, r, http.StatusBadRequest, "invalid_request", "invalid claim action", map[string]any{"field": "action"})
		return
	}
	claim.UpdatedAt = now

	writeJSON(w, http.StatusOK, map[string]any{
		"claim_id":     claim.ID,
		"field":        claim.Field,
		"label":        claim.Label,
		"status":       claim.Status,
		"review_state": claim.ReviewState,
		"updated_at":   isoTime(now),
	})
}

func (api *hunchAPI) handleCreateConversation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Mode               string `json:"mode"`
		Title              string `json:"title"`
		InitialUserMessage *struct {
			Text string `json:"text"`
		} `json:"initial_user_message"`
		ContextPolicy map[string]any `json:"context_policy"`
	}
	if !decodeRequestJSON(w, r, &req) {
		return
	}
	mode := req.Mode
	if mode == "" {
		mode = "reflection"
	}
	if !allowedValue(mode, []string{"onboarding", "reflection", "check_in", "memory_review"}) {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_request", "invalid conversation mode", map[string]any{"field": "mode"})
		return
	}

	api.store.mu.Lock()
	defer api.store.mu.Unlock()

	account := accountFromRequest(r)
	now := api.now()
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = defaultConversationTitle(mode)
	}
	conversation := &conversationState{
		ID:            "con_" + newID(),
		Mode:          mode,
		Title:         title,
		Status:        "active",
		Messages:      []*conversationMessage{},
		CreatedAt:     now,
		LastMessageAt: now,
	}
	if req.InitialUserMessage != nil && strings.TrimSpace(req.InitialUserMessage.Text) != "" {
		message := &conversationMessage{
			ID:        "msg_" + newID(),
			Role:      "user",
			Content:   map[string]any{"text": strings.TrimSpace(req.InitialUserMessage.Text)},
			CreatedAt: now,
		}
		conversation.Messages = append(conversation.Messages, message)
	}
	account.Conversations[conversation.ID] = conversation

	writeJSON(w, http.StatusCreated, map[string]any{
		"conversation_id": conversation.ID,
		"mode":            conversation.Mode,
		"title":           conversation.Title,
		"status":          conversation.Status,
		"created_at":      isoTime(conversation.CreatedAt),
		"messages":        conversationMessagesResponse(conversation.Messages),
		"next_action": map[string]any{
			"type":     "send_message",
			"endpoint": "/v1/conversations/" + conversation.ID + "/messages",
		},
	})
}

func (api *hunchAPI) handleListConversations(w http.ResponseWriter, r *http.Request) {
	api.store.mu.Lock()
	defer api.store.mu.Unlock()

	account := accountFromRequest(r)
	limit := parseLimit(r.URL.Query().Get("limit"), 20, 50)
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "active"
	}

	conversations := make([]*conversationState, 0, len(account.Conversations))
	for _, conversation := range account.Conversations {
		if status != "all" && conversation.Status != status {
			continue
		}
		conversations = append(conversations, conversation)
	}
	sort.Slice(conversations, func(i, j int) bool {
		return conversations[i].LastMessageAt.After(conversations[j].LastMessageAt)
	})
	if len(conversations) > limit {
		conversations = conversations[:limit]
	}

	items := make([]map[string]any, 0, len(conversations))
	for _, conversation := range conversations {
		items = append(items, map[string]any{
			"conversation_id": conversation.ID,
			"mode":            conversation.Mode,
			"title":           conversation.Title,
			"status":          conversation.Status,
			"last_message_at": isoTime(conversation.LastMessageAt),
			"created_at":      isoTime(conversation.CreatedAt),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items":       items,
		"next_cursor": nil,
	})
}

func (api *hunchAPI) handleGetConversation(w http.ResponseWriter, r *http.Request) {
	api.store.mu.Lock()
	defer api.store.mu.Unlock()

	account := accountFromRequest(r)
	conversation, ok := account.Conversations[chi.URLParam(r, "conversation_id")]
	if !ok {
		writeAPIError(w, r, http.StatusNotFound, "not_found", "conversation not found", nil)
		return
	}
	limit := parseLimit(r.URL.Query().Get("limit"), 30, 100)
	messages := conversation.Messages
	if len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"conversation_id": conversation.ID,
		"mode":            conversation.Mode,
		"title":           conversation.Title,
		"status":          conversation.Status,
		"messages":        conversationMessagesResponse(messages),
		"next_cursor":     nil,
	})
}

func (api *hunchAPI) handleSendConversationMessage(w http.ResponseWriter, r *http.Request) {
	result, ok := api.createConversationTurn(w, r)
	if !ok {
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"conversation_id":       result.ConversationID,
		"user_message":          conversationMessageResponse(result.UserMessage),
		"assistant_message":     conversationMessageResponse(result.AssistantMessage),
		"memory_update_preview": result.MemoryPreview,
		"suggested_replies":     result.SuggestedReplies,
	})
}

func (api *hunchAPI) handleStreamConversationMessage(w http.ResponseWriter, r *http.Request) {
	result, ok := api.createConversationTurn(w, r)
	if !ok {
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	writeSSE(w, "message.created", map[string]any{
		"message_id": result.AssistantMessage.ID,
		"role":       result.AssistantMessage.Role,
	})
	markdown, _ := result.AssistantMessage.Content["markdown"].(string)
	for _, chunk := range splitSSEChunks(markdown) {
		writeSSE(w, "content.delta", map[string]any{"text": chunk})
	}
	writeSSE(w, "memory.preview", result.MemoryPreview)
	writeSSE(w, "message.completed", map[string]any{
		"message_id": result.AssistantMessage.ID,
		"created_at": isoTime(result.AssistantMessage.CreatedAt),
	})
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (api *hunchAPI) handleConversationFeedback(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MessageID string   `json:"message_id"`
		Rating    string   `json:"rating"`
		Tags      []string `json:"tags"`
		Comment   string   `json:"comment"`
	}
	if !decodeRequestJSON(w, r, &req) {
		return
	}
	if !allowedValue(req.Rating, []string{"helpful", "not_helpful", "unsafe"}) {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_request", "invalid feedback rating", map[string]any{"field": "rating"})
		return
	}

	api.store.mu.Lock()
	defer api.store.mu.Unlock()

	account := accountFromRequest(r)
	conversation, ok := account.Conversations[chi.URLParam(r, "conversation_id")]
	if !ok {
		writeAPIError(w, r, http.StatusNotFound, "not_found", "conversation not found", nil)
		return
	}
	if !conversationHasMessage(conversation, req.MessageID) {
		writeAPIError(w, r, http.StatusNotFound, "not_found", "message not found for conversation", map[string]any{"field": "message_id"})
		return
	}

	now := api.now()
	feedback := &conversationFeedback{
		ID:             "fbk_" + newID(),
		ConversationID: conversation.ID,
		MessageID:      req.MessageID,
		Rating:         req.Rating,
		Tags:           cloneStrings(req.Tags),
		Comment:        strings.TrimSpace(req.Comment),
		CreatedAt:      now,
	}
	account.Feedback[feedback.ID] = feedback

	writeJSON(w, http.StatusCreated, map[string]any{
		"feedback_id": feedback.ID,
		"message_id":  feedback.MessageID,
		"created_at":  isoTime(feedback.CreatedAt),
	})
}

type conversationTurnResult struct {
	ConversationID   string
	UserMessage      *conversationMessage
	AssistantMessage *conversationMessage
	MemoryPreview    map[string]any
	SuggestedReplies []map[string]string
}

func (api *hunchAPI) createConversationTurn(w http.ResponseWriter, r *http.Request) (conversationTurnResult, bool) {
	var req struct {
		Message struct {
			Text string `json:"text"`
		} `json:"message"`
		ResponseOptions struct {
			Format                  string `json:"format"`
			IncludeMemoryPreview    bool   `json:"include_memory_preview"`
			IncludeSuggestedReplies bool   `json:"include_suggested_replies"`
		} `json:"response_options"`
		ContextPolicy map[string]any `json:"context_policy"`
	}
	if !decodeRequestJSON(w, r, &req) {
		return conversationTurnResult{}, false
	}
	text := strings.TrimSpace(req.Message.Text)
	if text == "" || len(text) > 4000 {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_request", "message.text is required and must be at most 4000 characters", map[string]any{"field": "message.text"})
		return conversationTurnResult{}, false
	}

	api.store.mu.Lock()
	defer api.store.mu.Unlock()

	account := accountFromRequest(r)
	conversation, ok := account.Conversations[chi.URLParam(r, "conversation_id")]
	if !ok {
		writeAPIError(w, r, http.StatusNotFound, "not_found", "conversation not found", nil)
		return conversationTurnResult{}, false
	}
	if conversation.Status != "active" {
		writeAPIError(w, r, http.StatusConflict, "conflict", "conversation is not active", nil)
		return conversationTurnResult{}, false
	}

	now := api.now()
	userMessage := &conversationMessage{
		ID:        "msg_" + newID(),
		Role:      "user",
		Content:   map[string]any{"text": text},
		CreatedAt: now,
	}

	assistantContent, metadata := buildAssistantReflection(account, text)
	assistantMessage := &conversationMessage{
		ID:        "msg_" + newID(),
		Role:      "assistant",
		Content:   assistantContent,
		Metadata:  metadata,
		CreatedAt: now.Add(1 * time.Second),
	}
	conversation.Messages = append(conversation.Messages, userMessage, assistantMessage)
	conversation.LastMessageAt = assistantMessage.CreatedAt

	memoryPreview := memoryPreviewFor(account, text)
	suggestedReplies := []map[string]string{}
	if req.ResponseOptions.IncludeSuggestedReplies {
		suggestedReplies = []map[string]string{
			{"text": "Help me define my stability floor."},
			{"text": "Ask me a question that clarifies what freedom means here."},
		}
	}

	return conversationTurnResult{
		ConversationID:   conversation.ID,
		UserMessage:      userMessage,
		AssistantMessage: assistantMessage,
		MemoryPreview:    memoryPreview,
		SuggestedReplies: suggestedReplies,
	}, true
}

func (api *hunchAPI) bootstrapResponse(account *hunchAccount) map[string]any {
	setupRequired := account.Profile.SetupStatus == "not_started"
	activeSetupSessionID := nullableString(account.ActiveSetupSessionID)
	nextStep := "none"
	if setupRequired {
		nextStep = "create_setup_session"
		if account.ActiveSetupSessionID != "" {
			nextStep = "continue_setup"
		}
	} else if needsReviewCount(account.Claims) > 0 {
		nextStep = "review_memory"
	} else {
		nextStep = "start_conversation"
	}

	return map[string]any{
		"account_id": account.AccountID,
		"profile": map[string]any{
			"user_id":            account.UserID,
			"display_name":       nullableStringPtr(account.Profile.DisplayName),
			"locale":             account.Profile.Locale,
			"timezone":           account.Profile.Timezone,
			"setup_status":       account.Profile.SetupStatus,
			"setup_completed_at": nullableTime(account.Profile.SetupCompletedAt),
		},
		"setup": map[string]any{
			"required":                setupRequired,
			"active_setup_session_id": activeSetupSessionID,
			"next_step":               nextStep,
			"completion_ratio":        bootstrapCompletionRatio(account),
		},
		"self_model": map[string]any{
			"snapshot_id":        nullableString(account.SelfModelSnapshotID),
			"claim_count":        activeClaimCount(account.Claims),
			"last_updated_at":    latestClaimUpdate(account.Claims),
			"needs_review_count": needsReviewCount(account.Claims),
		},
		"conversation": map[string]any{
			"suggested_start_mode":   suggestedStartMode(account),
			"latest_conversation_id": nullableString(latestConversationID(account.Conversations)),
		},
	}
}

func setupSessionResponse(session *setupSessionState) map[string]any {
	answers := make([]map[string]any, 0, len(session.Answers))
	for _, step := range setupSteps {
		answer := session.Answers[step.StepID]
		if answer == nil {
			continue
		}
		answers = append(answers, map[string]any{
			"step_id":     answer.StepID,
			"answer":      answer.Answer,
			"answered_at": isoTime(answer.AnsweredAt),
		})
	}

	return map[string]any{
		"setup_session_id": session.ID,
		"status":           session.Status,
		"answers":          answers,
		"current_step":     currentSetupStep(session),
		"progress":         setupProgress(session),
	}
}

func currentSetupStep(session *setupSessionState) any {
	if session.Status != "in_progress" {
		return nil
	}
	for _, step := range setupSteps {
		if _, ok := session.Answers[step.StepID]; !ok {
			stepCopy := step
			return stepCopy
		}
	}
	return nil
}

func setupProgress(session *setupSessionState) map[string]any {
	total := len(setupSteps)
	completed := len(session.Answers)
	ratio := 0.0
	if total > 0 {
		ratio = float64(completed) / float64(total)
	}
	return map[string]any{
		"completed_steps":  completed,
		"total_steps":      total,
		"completion_ratio": roundRatio(ratio),
	}
}

func validateSetupAnswer(session *setupSessionState, stepID string, answer map[string]any) error {
	step, ok := setupStepByID(stepID)
	if !ok {
		return fmt.Errorf("unknown setup step")
	}
	if current := currentSetupStep(session); current != nil {
		currentStep := current.(setupStep)
		if stepID != currentStep.StepID {
			if _, replacing := session.Answers[stepID]; !replacing {
				return fmt.Errorf("step_id must match the current step")
			}
		}
	}

	switch step.StepID {
	case "display_name":
		text := strings.TrimSpace(stringFromAny(answer["text"]))
		if len(text) < 1 || len(text) > 40 {
			return fmt.Errorf("display name must be 1-40 characters")
		}
	case "focus_areas":
		values := stringsFromAny(answer["values"])
		if len(values) < 1 || len(values) > 4 {
			return fmt.Errorf("focus_areas must include 1-4 values")
		}
		for _, value := range values {
			if !allowedValue(value, []string{"career", "relationships", "self_understanding", "faith"}) {
				return fmt.Errorf("focus_areas contains an invalid value")
			}
		}
	case "current_context":
		text := strings.TrimSpace(stringFromAny(answer["text"]))
		if text == "" || len(text) > 2000 {
			return fmt.Errorf("current_context text must be 1-2000 characters")
		}
	case "conversation_style":
		value := stringFromAny(answer["value"])
		if !allowedValue(value, []string{"gentle_structured", "direct_practical", "deep_reflective"}) {
			return fmt.Errorf("conversation_style is invalid")
		}
	case "values_and_boundaries":
		if len(stringsFromAny(answer["values"])) > 8 {
			return fmt.Errorf("values_and_boundaries supports at most 8 values")
		}
	case "memory_consent":
		if _, ok := answer["memory_opt_in"].(bool); !ok {
			return fmt.Errorf("memory_opt_in is required")
		}
		if _, ok := answer["review_before_use"].(bool); !ok {
			return fmt.Errorf("review_before_use is required")
		}
	}
	return nil
}

func setupStepByID(stepID string) (setupStep, bool) {
	for _, step := range setupSteps {
		if step.StepID == stepID {
			return step, true
		}
	}
	return setupStep{}, false
}

func missingRequiredSetupSteps(session *setupSessionState) []string {
	missing := []string{}
	for _, step := range setupSteps {
		if !step.Required {
			continue
		}
		if _, ok := session.Answers[step.StepID]; !ok {
			missing = append(missing, step.StepID)
		}
	}
	return missing
}

func applySetupAnswersToProfile(account *hunchAccount, session *setupSessionState, now time.Time) {
	if answer := session.Answers["display_name"]; answer != nil {
		name := strings.TrimSpace(stringFromAny(answer.Answer["text"]))
		account.Profile.DisplayName = &name
	}
	if session.ClientContext.Locale != "" {
		account.Profile.Locale = session.ClientContext.Locale
	}
	if session.ClientContext.Timezone != "" {
		account.Profile.Timezone = session.ClientContext.Timezone
	}
	if answer := session.Answers["focus_areas"]; answer != nil {
		account.Profile.FocusAreas = stringsFromAny(answer.Answer["values"])
	}
	if answer := session.Answers["conversation_style"]; answer != nil {
		account.Profile.ConversationStyle = styleFromSetupValue(stringFromAny(answer.Answer["value"]))
	}
	if answer := session.Answers["values_and_boundaries"]; answer != nil {
		account.Profile.Boundaries.AvoidTopics = stringsFromAny(answer.Answer["avoid_topics"])
		account.Profile.Boundaries.SensitiveTopicsOK = stringsFromAny(answer.Answer["sensitive_topics_ok"])
	}
	if answer := session.Answers["memory_consent"]; answer != nil {
		account.Profile.Boundaries.MemoryOptIn = boolFromAny(answer.Answer["memory_opt_in"])
		session.MemoryOptIn = account.Profile.Boundaries.MemoryOptIn
		session.ReviewBeforeUse = boolFromAny(answer.Answer["review_before_use"])
	}
	account.Profile.SetupStatus = "completed"
	account.Profile.SetupCompletedAt = &now
	account.Profile.UpdatedAt = now
	account.Profile.Version++
}

func styleFromSetupValue(value string) conversationStyle {
	switch value {
	case "direct_practical":
		return conversationStyle{Depth: "balanced", Directness: "direct", DefaultLanguage: "ko", ReflectionFormat: "structured"}
	case "deep_reflective":
		return conversationStyle{Depth: "deep", Directness: "gentle", DefaultLanguage: "ko", ReflectionFormat: "structured"}
	default:
		return conversationStyle{Depth: "balanced", Directness: "gentle", DefaultLanguage: "ko", ReflectionFormat: "structured"}
	}
}

func seedSelfModelClaims(account *hunchAccount, session *setupSessionState, now time.Time) {
	if account.SelfModelSnapshotID == "" {
		account.SelfModelSnapshotID = "sms_" + newID()
	}
	date := now.Format("2006-01-02")

	if answer := session.Answers["current_context"]; answer != nil {
		text := stringFromAny(answer.Answer["text"])
		label := "Is exploring how to balance stability and creative work"
		if strings.TrimSpace(text) != "" {
			label = summarizeContextClaim(text)
		}
		addClaimIfMissing(account, &selfModelClaim{
			ID:          "clm_" + newID(),
			Field:       "core_desires",
			Label:       label,
			Source:      "user_declared",
			EvidenceIDs: []string{answer.ID},
			SourceTypes: []string{"setup_answer"},
			Confidence:  0.82,
			FirstSeen:   date,
			LastSeen:    date,
			Stability:   "candidate",
			Status:      "active",
			ReviewState: "unreviewed",
			UpdatedAt:   now,
		})
	}
	if answer := session.Answers["values_and_boundaries"]; answer != nil {
		for _, value := range stringsFromAny(answer.Answer["values"]) {
			addClaimIfMissing(account, &selfModelClaim{
				ID:          "clm_" + newID(),
				Field:       "values",
				Label:       "Values " + value,
				Source:      "user_declared",
				EvidenceIDs: []string{answer.ID},
				SourceTypes: []string{"setup_answer"},
				Confidence:  0.80,
				FirstSeen:   date,
				LastSeen:    date,
				Stability:   "candidate",
				Status:      "active",
				ReviewState: "unreviewed",
				UpdatedAt:   now,
			})
		}
	}
}

func addClaimIfMissing(account *hunchAccount, claim *selfModelClaim) {
	for _, existing := range account.Claims {
		if existing.Field == claim.Field && strings.EqualFold(existing.Label, claim.Label) && existing.Status == "active" {
			return
		}
	}
	account.Claims[claim.ID] = claim
}

func completeSetupResponse(account *hunchAccount, session *setupSessionState) map[string]any {
	claims := make([]map[string]any, 0)
	for _, claim := range sortedClaims(account.Claims) {
		if claim.Status != "active" {
			continue
		}
		claims = append(claims, map[string]any{
			"claim_id":     claim.ID,
			"field":        claim.Field,
			"label":        claim.Label,
			"source":       claim.Source,
			"confidence":   claim.Confidence,
			"review_state": claim.ReviewState,
		})
	}

	return map[string]any{
		"setup_session_id": session.ID,
		"status":           session.Status,
		"profile": map[string]any{
			"user_id":            account.UserID,
			"display_name":       nullableStringPtr(account.Profile.DisplayName),
			"locale":             account.Profile.Locale,
			"timezone":           account.Profile.Timezone,
			"setup_status":       account.Profile.SetupStatus,
			"setup_completed_at": nullableTime(account.Profile.SetupCompletedAt),
		},
		"self_model": map[string]any{
			"snapshot_id":        nullableString(account.SelfModelSnapshotID),
			"candidate_claims":   claims,
			"needs_review_count": needsReviewCount(account.Claims),
		},
		"next_action": map[string]any{
			"type":           "start_conversation",
			"suggested_mode": "reflection",
			"starter_prompt": "Tell me one question you most want to sort out right now.",
		},
	}
}

func profileResponse(account *hunchAccount) map[string]any {
	return map[string]any{
		"user_id":            account.UserID,
		"display_name":       nullableStringPtr(account.Profile.DisplayName),
		"locale":             account.Profile.Locale,
		"timezone":           account.Profile.Timezone,
		"conversation_style": account.Profile.ConversationStyle,
		"focus_areas":        account.Profile.FocusAreas,
		"boundaries":         account.Profile.Boundaries,
		"version":            account.Profile.Version,
		"updated_at":         isoTime(account.Profile.UpdatedAt),
	}
}

func mergeConversationStyle(dst *conversationStyle, src conversationStyle) {
	if src.Depth != "" {
		dst.Depth = src.Depth
	}
	if src.Directness != "" {
		dst.Directness = src.Directness
	}
	if src.DefaultLanguage != "" {
		dst.DefaultLanguage = src.DefaultLanguage
	}
	if src.ReflectionFormat != "" {
		dst.ReflectionFormat = src.ReflectionFormat
	}
}

func buildAssistantReflection(account *hunchAccount, userText string) (map[string]any, map[string]any) {
	claimIDs := activeUsableClaimIDs(account.Claims, account.Profile.ReviewBeforeUse)
	evidenceIDs := evidenceIDsForClaims(account.Claims, claimIDs)
	sections := []map[string]any{
		{
			"type":  "what_you_want",
			"title": "What You Want",
			"items": []string{"You want more room to choose deliberately instead of reacting to pressure."},
		},
		{
			"type":  "current_reality",
			"title": "Current Reality",
			"items": []string{"The concern you named is concrete: money, regret, and stability all shape the choice."},
		},
		{
			"type":  "hidden_tension",
			"title": "Hidden Tension",
			"items": []string{"This looks less like freedom versus fear and more like autonomy versus the need for a reliable floor."},
		},
		{
			"type":  "reframing",
			"title": "Reframing",
			"items": []string{"The useful question may be what minimum stability would let you experiment without panic."},
		},
		{
			"type":  "next_small_step",
			"title": "Next Small Step (24-72h)",
			"items": []string{"Write a one-page stability floor: minimum monthly income, runway, and the smallest creative experiment that fits beside it."},
		},
	}
	markdown := sectionsToMarkdown(sections)
	if isSafetySensitive(userText) {
		sections[1]["items"] = []string{"This sounds intense enough that the safest next step is to involve a trusted person or local emergency support now."}
		markdown = sectionsToMarkdown(sections)
	}

	return map[string]any{
			"format":   "hunch_reflection_v0",
			"markdown": markdown,
			"sections": sections,
		}, map[string]any{
			"evidence_ids_used":         evidenceIDs,
			"self_model_claim_ids_used": claimIDs,
			"risk_flags": map[string]bool{
				"negativity_reinforcement": isSafetySensitive(userText),
				"value_overreach":          false,
				"false_personalization":    false,
			},
		}
}

func memoryPreviewFor(account *hunchAccount, text string) map[string]any {
	if !account.Profile.Boundaries.MemoryOptIn {
		return map[string]any{
			"status":           "disabled",
			"candidate_claims": []map[string]any{},
		}
	}

	return map[string]any{
		"status": "queued",
		"candidate_claims": []map[string]any{
			{
				"field":      "recurring_conflicts",
				"label":      inferConversationClaim(text),
				"source":     "model_inferred",
				"confidence": 0.58,
				"stability":  "candidate",
			},
		},
	}
}

func conversationMessagesResponse(messages []*conversationMessage) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		out = append(out, conversationMessageResponse(message))
	}
	return out
}

func conversationMessageResponse(message *conversationMessage) map[string]any {
	out := map[string]any{
		"message_id": message.ID,
		"role":       message.Role,
		"content":    message.Content,
		"created_at": isoTime(message.CreatedAt),
	}
	if len(message.Metadata) > 0 {
		out["metadata"] = message.Metadata
	}
	return out
}

func selfModelClaimResponse(claim *selfModelClaim) map[string]any {
	return map[string]any{
		"claim_id":     claim.ID,
		"field":        claim.Field,
		"label":        claim.Label,
		"source":       claim.Source,
		"evidence_ids": claim.EvidenceIDs,
		"source_types": claim.SourceTypes,
		"confidence":   claim.Confidence,
		"stability":    claim.Stability,
		"status":       claim.Status,
		"review_state": claim.ReviewState,
		"first_seen":   claim.FirstSeen,
		"last_seen":    claim.LastSeen,
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeAPIError(w http.ResponseWriter, r *http.Request, status int, code, message string, details map[string]any) {
	if details == nil {
		details = map[string]any{}
	}
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":       code,
			"message":    message,
			"details":    details,
			"request_id": requestID(r),
		},
	})
}

func decodeRequestJSON(w http.ResponseWriter, r *http.Request, out any) bool {
	if r.Body == nil {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_request", "request body is required", nil)
		return false
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := decoder.Decode(out); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_request", "malformed JSON request", nil)
		return false
	}
	return true
}

func writeSSE(w http.ResponseWriter, event string, data any) {
	raw, err := json.Marshal(data)
	if err != nil {
		raw = []byte(`{"error":"encode"}`)
	}
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, raw)
}

func requestID(r *http.Request) string {
	if id := middleware.GetReqID(r.Context()); id != "" {
		return id
	}
	return "req_" + newID()
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b[:])
}

func isoTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func nullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return isoTime(*t)
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableStringPtr(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func roundRatio(value float64) float64 {
	return float64(int(value*100+0.5)) / 100
}

func cloneObject(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneStrings(in []string) []string {
	if in == nil {
		return []string{}
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func stringsFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return cloneStrings(typed)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if str, ok := item.(string); ok && strings.TrimSpace(str) != "" {
				out = append(out, strings.TrimSpace(str))
			}
		}
		return out
	default:
		return []string{}
	}
}

func stringFromAny(value any) string {
	str, _ := value.(string)
	return str
}

func boolFromAny(value any) bool {
	boolean, _ := value.(bool)
	return boolean
}

func allowedValue(value string, allowed []string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func splitFilter(raw string) map[string]bool {
	if raw == "" {
		return nil
	}
	out := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out[part] = true
		}
	}
	return out
}

func sortedClaims(claims map[string]*selfModelClaim) []*selfModelClaim {
	out := make([]*selfModelClaim, 0, len(claims))
	for _, claim := range claims {
		out = append(out, claim)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].UpdatedAt.Before(out[j].UpdatedAt)
	})
	return out
}

func latestClaimUpdate(claims map[string]*selfModelClaim) any {
	var latest time.Time
	for _, claim := range claims {
		if claim.UpdatedAt.After(latest) {
			latest = claim.UpdatedAt
		}
	}
	if latest.IsZero() {
		return nil
	}
	return isoTime(latest)
}

func needsReviewCount(claims map[string]*selfModelClaim) int {
	count := 0
	for _, claim := range claims {
		if claim.Status == "active" && claim.ReviewState == "unreviewed" {
			count++
		}
	}
	return count
}

func activeClaimCount(claims map[string]*selfModelClaim) int {
	count := 0
	for _, claim := range claims {
		if claim.Status == "active" {
			count++
		}
	}
	return count
}

func activeUsableClaimIDs(claims map[string]*selfModelClaim, requireReviewed bool) []string {
	ids := []string{}
	for _, claim := range sortedClaims(claims) {
		if claim.Status != "active" || claim.ReviewState == "hidden" || claim.ReviewState == "rejected" {
			continue
		}
		if requireReviewed && claim.ReviewState != "confirmed" && claim.ReviewState != "corrected" {
			continue
		}
		ids = append(ids, claim.ID)
	}
	return ids
}

func evidenceIDsForClaims(claims map[string]*selfModelClaim, claimIDs []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, claimID := range claimIDs {
		claim := claims[claimID]
		if claim == nil {
			continue
		}
		for _, evidenceID := range claim.EvidenceIDs {
			if seen[evidenceID] {
				continue
			}
			seen[evidenceID] = true
			out = append(out, evidenceID)
		}
	}
	return out
}

func latestConversationID(conversations map[string]*conversationState) string {
	var latest *conversationState
	for _, conversation := range conversations {
		if latest == nil || conversation.LastMessageAt.After(latest.LastMessageAt) {
			latest = conversation
		}
	}
	if latest == nil {
		return ""
	}
	return latest.ID
}

func suggestedStartMode(account *hunchAccount) string {
	if account.Profile.SetupStatus == "completed" {
		return "reflection"
	}
	return "onboarding"
}

func bootstrapCompletionRatio(account *hunchAccount) float64 {
	if account.Profile.SetupStatus == "completed" {
		return 1
	}
	if account.ActiveSetupSessionID == "" {
		return 0
	}
	session := account.SetupSessions[account.ActiveSetupSessionID]
	if session == nil {
		return 0
	}
	progress := setupProgress(session)
	ratio, _ := progress["completion_ratio"].(float64)
	return ratio
}

func parseLimit(raw string, fallback, max int) int {
	if raw == "" {
		return fallback
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return fallback
	}
	if limit > max {
		return max
	}
	return limit
}

func defaultConversationTitle(mode string) string {
	switch mode {
	case "onboarding":
		return "Getting started"
	case "check_in":
		return "Check-in"
	case "memory_review":
		return "Memory review"
	default:
		return "Reflection"
	}
}

func conversationHasMessage(conversation *conversationState, messageID string) bool {
	for _, message := range conversation.Messages {
		if message.ID == messageID {
			return true
		}
	}
	return false
}

func sectionsToMarkdown(sections []map[string]any) string {
	parts := make([]string, 0, len(sections))
	for _, section := range sections {
		title, _ := section["title"].(string)
		items, _ := section["items"].([]string)
		lines := []string{"## " + title}
		for _, item := range items {
			lines = append(lines, "- "+item)
		}
		parts = append(parts, strings.Join(lines, "\n"))
	}
	return strings.Join(parts, "\n\n")
}

func summarizeContextClaim(text string) string {
	lowered := strings.ToLower(text)
	if strings.Contains(lowered, "stability") && (strings.Contains(lowered, "creative") || strings.Contains(lowered, "autonomy")) {
		return "Wants creative autonomy while preserving basic stability"
	}
	if len(text) > 120 {
		text = text[:120]
	}
	return "Named current context: " + strings.TrimSpace(text)
}

func inferConversationClaim(text string) string {
	lowered := strings.ToLower(text)
	if strings.Contains(lowered, "freedom") || strings.Contains(lowered, "creative") {
		return "Creative autonomy versus reliable stability"
	}
	return "A current decision may involve competing values"
}

func isSafetySensitive(text string) bool {
	lowered := strings.ToLower(text)
	return strings.Contains(lowered, "kill myself") ||
		strings.Contains(lowered, "suicide") ||
		strings.Contains(lowered, "hurt myself")
}

func splitSSEChunks(text string) []string {
	if text == "" {
		return []string{""}
	}
	lines := strings.SplitAfter(text, "\n")
	chunks := make([]string, 0, len(lines))
	for _, line := range lines {
		if line != "" {
			chunks = append(chunks, line)
		}
	}
	return chunks
}
