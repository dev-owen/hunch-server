package httpserver

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestV1EndpointsRequireHunchSession(t *testing.T) {
	router := NewRouter(Dependencies{})

	req := httptest.NewRequest(http.MethodGet, "/v1/me/bootstrap", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want application/json; charset=utf-8", got)
	}

	var body map[string]any
	decodeJSON(t, rec.Body, &body)
	errBody := body["error"].(map[string]any)
	if errBody["code"] != "unauthenticated" {
		t.Fatalf("error.code = %v, want unauthenticated", errBody["code"])
	}
	if errBody["request_id"] == "" {
		t.Fatal("error.request_id is empty")
	}
}

func TestSetupFlowCompletesProfileAndSelfModel(t *testing.T) {
	router := NewRouter(Dependencies{})

	bootstrap := doJSON(t, router, http.MethodGet, "/v1/me/bootstrap", nil)
	if bootstrap.Code != http.StatusOK {
		t.Fatalf("bootstrap status = %d, want %d; body=%s", bootstrap.Code, http.StatusOK, bootstrap.Body.String())
	}
	var bootstrapBody map[string]any
	decodeJSON(t, bootstrap.Body, &bootstrapBody)
	setup := bootstrapBody["setup"].(map[string]any)
	if setup["required"] != true {
		t.Fatalf("setup.required = %v, want true", setup["required"])
	}
	if setup["next_step"] != "create_setup_session" {
		t.Fatalf("setup.next_step = %v, want create_setup_session", setup["next_step"])
	}

	sessionID := startSetup(t, router)
	answerSetup(t, router, sessionID, "display_name", map[string]any{"text": "Owen"})
	answerSetup(t, router, sessionID, "focus_areas", map[string]any{"values": []string{"career", "faith"}})
	answerSetup(t, router, sessionID, "current_context", map[string]any{
		"text": "I am deciding how to balance stability and creative work.",
	})
	answerSetup(t, router, sessionID, "conversation_style", map[string]any{"value": "gentle_structured"})
	answerSetup(t, router, sessionID, "values_and_boundaries", map[string]any{
		"values":              []string{"integrity", "creative autonomy"},
		"sensitive_topics_ok": []string{"faith"},
		"avoid_topics":        []string{"diet talk"},
	})
	answerSetup(t, router, sessionID, "memory_consent", map[string]any{
		"memory_opt_in":     true,
		"review_before_use": true,
	})

	complete := doJSON(t, router, http.MethodPost, "/v1/me/setup/sessions/"+sessionID+"/complete", map[string]any{
		"memory_opt_in":     true,
		"review_before_use": true,
	})
	if complete.Code != http.StatusOK {
		t.Fatalf("complete status = %d, want %d; body=%s", complete.Code, http.StatusOK, complete.Body.String())
	}
	var completeBody map[string]any
	decodeJSON(t, complete.Body, &completeBody)
	profile := completeBody["profile"].(map[string]any)
	if profile["display_name"] != "Owen" {
		t.Fatalf("profile.display_name = %v, want Owen", profile["display_name"])
	}
	if profile["setup_status"] != "completed" {
		t.Fatalf("profile.setup_status = %v, want completed", profile["setup_status"])
	}

	selfModel := completeBody["self_model"].(map[string]any)
	claims := selfModel["candidate_claims"].([]any)
	if len(claims) == 0 {
		t.Fatal("candidate_claims is empty")
	}
	firstClaim := claims[0].(map[string]any)
	if firstClaim["source"] != "user_declared" {
		t.Fatalf("claim.source = %v, want user_declared", firstClaim["source"])
	}
	if firstClaim["review_state"] != "unreviewed" {
		t.Fatalf("claim.review_state = %v, want unreviewed", firstClaim["review_state"])
	}

	after := doJSON(t, router, http.MethodGet, "/v1/me/bootstrap", nil)
	if after.Code != http.StatusOK {
		t.Fatalf("bootstrap after setup status = %d, want %d", after.Code, http.StatusOK)
	}
	var afterBody map[string]any
	decodeJSON(t, after.Body, &afterBody)
	afterSetup := afterBody["setup"].(map[string]any)
	if afterSetup["required"] != false {
		t.Fatalf("setup.required after completion = %v, want false", afterSetup["required"])
	}
	if afterSetup["next_step"] != "review_memory" {
		t.Fatalf("setup.next_step after completion = %v, want review_memory", afterSetup["next_step"])
	}

	claimID := firstClaim["claim_id"].(string)
	patch := doJSON(t, router, http.MethodPatch, "/v1/me/self-model/claims/"+claimID, map[string]any{
		"action":          "correct",
		"corrected_label": "I want creative autonomy, but not at the cost of basic stability.",
		"note":            "More precise.",
	})
	if patch.Code != http.StatusOK {
		t.Fatalf("patch claim status = %d, want %d; body=%s", patch.Code, http.StatusOK, patch.Body.String())
	}
	var patchBody map[string]any
	decodeJSON(t, patch.Body, &patchBody)
	if patchBody["review_state"] != "corrected" {
		t.Fatalf("patched review_state = %v, want corrected", patchBody["review_state"])
	}
}

func TestProfilePatchRejectsStaleVersion(t *testing.T) {
	router := NewRouter(Dependencies{})
	completeSetup(t, router)

	get := doJSON(t, router, http.MethodGet, "/v1/me/profile", nil)
	if get.Code != http.StatusOK {
		t.Fatalf("profile status = %d, want %d", get.Code, http.StatusOK)
	}
	var profile map[string]any
	decodeJSON(t, get.Body, &profile)
	version := int(profile["version"].(float64))

	patch := doJSON(t, router, http.MethodPatch, "/v1/me/profile", map[string]any{
		"display_name": "Owen K",
		"version":      version,
	})
	if patch.Code != http.StatusOK {
		t.Fatalf("profile patch status = %d, want %d; body=%s", patch.Code, http.StatusOK, patch.Body.String())
	}

	stale := doJSON(t, router, http.MethodPatch, "/v1/me/profile", map[string]any{
		"display_name": "Old Version",
		"version":      version,
	})
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale profile patch status = %d, want %d; body=%s", stale.Code, http.StatusConflict, stale.Body.String())
	}
	var staleBody map[string]any
	decodeJSON(t, stale.Body, &staleBody)
	if staleBody["error"].(map[string]any)["code"] != "conflict" {
		t.Fatalf("error.code = %v, want conflict", staleBody["error"].(map[string]any)["code"])
	}
}

func TestConversationFlowProducesReflectionContract(t *testing.T) {
	router := NewRouter(Dependencies{})
	completeSetup(t, router)

	create := doJSON(t, router, http.MethodPost, "/v1/conversations", map[string]any{
		"mode":  "reflection",
		"title": "Career stability vs creative autonomy",
		"initial_user_message": map[string]any{
			"text": "I keep going back and forth between a stable job and building my own product.",
		},
		"context_policy": map[string]any{
			"use_self_model":           true,
			"use_recent_conversations": true,
			"use_setup_answers":        true,
		},
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("create conversation status = %d, want %d; body=%s", create.Code, http.StatusCreated, create.Body.String())
	}
	var createBody map[string]any
	decodeJSON(t, create.Body, &createBody)
	conversationID := createBody["conversation_id"].(string)
	if conversationID == "" {
		t.Fatal("conversation_id is empty")
	}

	send := doJSON(t, router, http.MethodPost, "/v1/conversations/"+conversationID+"/messages", map[string]any{
		"message": map[string]any{
			"text": "I want freedom, but I am scared I will regret not choosing money.",
		},
		"response_options": map[string]any{
			"format":                    "hunch_reflection_v0",
			"include_memory_preview":    true,
			"include_suggested_replies": true,
		},
	})
	if send.Code != http.StatusOK {
		t.Fatalf("send message status = %d, want %d; body=%s", send.Code, http.StatusOK, send.Body.String())
	}
	var sendBody map[string]any
	decodeJSON(t, send.Body, &sendBody)
	assistant := sendBody["assistant_message"].(map[string]any)
	content := assistant["content"].(map[string]any)
	if content["format"] != "hunch_reflection_v0" {
		t.Fatalf("assistant format = %v, want hunch_reflection_v0", content["format"])
	}
	sections := content["sections"].([]any)
	if len(sections) != 5 {
		t.Fatalf("sections len = %d, want 5", len(sections))
	}
	wantTypes := []string{"what_you_want", "current_reality", "hidden_tension", "reframing", "next_small_step"}
	for i, want := range wantTypes {
		got := sections[i].(map[string]any)["type"]
		if got != want {
			t.Fatalf("section[%d].type = %v, want %s", i, got, want)
		}
	}
	preview := sendBody["memory_update_preview"].(map[string]any)
	if preview["status"] != "queued" {
		t.Fatalf("memory_update_preview.status = %v, want queued", preview["status"])
	}
	if len(sendBody["suggested_replies"].([]any)) == 0 {
		t.Fatal("suggested_replies is empty")
	}

	list := doJSON(t, router, http.MethodGet, "/v1/conversations", nil)
	if list.Code != http.StatusOK {
		t.Fatalf("list conversations status = %d, want %d", list.Code, http.StatusOK)
	}
	var listBody map[string]any
	decodeJSON(t, list.Body, &listBody)
	if len(listBody["items"].([]any)) != 1 {
		t.Fatalf("list items len = %d, want 1", len(listBody["items"].([]any)))
	}

	stream := doJSON(t, router, http.MethodPost, "/v1/conversations/"+conversationID+"/messages/stream", map[string]any{
		"message": map[string]any{"text": "Stream this reflection too."},
	})
	if stream.Code != http.StatusOK {
		t.Fatalf("stream status = %d, want %d; body=%s", stream.Code, http.StatusOK, stream.Body.String())
	}
	if got := stream.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("stream Content-Type = %q, want text/event-stream", got)
	}
	if !strings.Contains(stream.Body.String(), "event: message.completed") {
		t.Fatalf("stream body missing message.completed event: %s", stream.Body.String())
	}

	messageID := assistant["message_id"].(string)
	feedback := doJSON(t, router, http.MethodPost, "/v1/conversations/"+conversationID+"/feedback", map[string]any{
		"message_id": messageID,
		"rating":     "helpful",
		"tags":       []string{"felt_personal", "actionable"},
		"comment":    "The stability floor idea was useful.",
	})
	if feedback.Code != http.StatusCreated {
		t.Fatalf("feedback status = %d, want %d; body=%s", feedback.Code, http.StatusCreated, feedback.Body.String())
	}
	var feedbackBody map[string]any
	decodeJSON(t, feedback.Body, &feedbackBody)
	if feedbackBody["feedback_id"] == "" {
		t.Fatal("feedback_id is empty")
	}
}

func TestReviewBeforeUseExcludesUnreviewedClaimsFromPersonalization(t *testing.T) {
	router := NewRouter(Dependencies{})

	sessionID := startSetup(t, router)
	answerSetup(t, router, sessionID, "display_name", map[string]any{"text": "Owen"})
	answerSetup(t, router, sessionID, "focus_areas", map[string]any{"values": []string{"career"}})
	answerSetup(t, router, sessionID, "current_context", map[string]any{
		"text": "I am deciding how to balance stability and creative work.",
	})
	answerSetup(t, router, sessionID, "conversation_style", map[string]any{"value": "gentle_structured"})
	answerSetup(t, router, sessionID, "values_and_boundaries", map[string]any{
		"values":              []string{"integrity"},
		"sensitive_topics_ok": []string{},
		"avoid_topics":        []string{},
	})
	answerSetup(t, router, sessionID, "memory_consent", map[string]any{
		"memory_opt_in":     true,
		"review_before_use": true,
	})

	complete := doJSON(t, router, http.MethodPost, "/v1/me/setup/sessions/"+sessionID+"/complete", map[string]any{
		"memory_opt_in":     true,
		"review_before_use": true,
	})
	if complete.Code != http.StatusOK {
		t.Fatalf("complete setup status = %d, want %d; body=%s", complete.Code, http.StatusOK, complete.Body.String())
	}
	var completeBody map[string]any
	decodeJSON(t, complete.Body, &completeBody)
	claims := completeBody["self_model"].(map[string]any)["candidate_claims"].([]any)
	claimID := claims[0].(map[string]any)["claim_id"].(string)

	conversationID := createConversation(t, router)
	send := doJSON(t, router, http.MethodPost, "/v1/conversations/"+conversationID+"/messages", map[string]any{
		"message": map[string]any{"text": "What should I do with this career question?"},
	})
	if send.Code != http.StatusOK {
		t.Fatalf("send message status = %d, want %d; body=%s", send.Code, http.StatusOK, send.Body.String())
	}
	var sendBody map[string]any
	decodeJSON(t, send.Body, &sendBody)
	metadata := sendBody["assistant_message"].(map[string]any)["metadata"].(map[string]any)
	if got := len(metadata["self_model_claim_ids_used"].([]any)); got != 0 {
		t.Fatalf("self_model_claim_ids_used len = %d, want 0 before review", got)
	}

	confirm := doJSON(t, router, http.MethodPatch, "/v1/me/self-model/claims/"+claimID, map[string]any{
		"action": "confirm",
	})
	if confirm.Code != http.StatusOK {
		t.Fatalf("confirm claim status = %d, want %d; body=%s", confirm.Code, http.StatusOK, confirm.Body.String())
	}

	afterConfirm := doJSON(t, router, http.MethodPost, "/v1/conversations/"+conversationID+"/messages", map[string]any{
		"message": map[string]any{"text": "Now help me reflect with reviewed memory."},
	})
	if afterConfirm.Code != http.StatusOK {
		t.Fatalf("send after confirm status = %d, want %d; body=%s", afterConfirm.Code, http.StatusOK, afterConfirm.Body.String())
	}
	var afterConfirmBody map[string]any
	decodeJSON(t, afterConfirm.Body, &afterConfirmBody)
	afterMetadata := afterConfirmBody["assistant_message"].(map[string]any)["metadata"].(map[string]any)
	if got := len(afterMetadata["self_model_claim_ids_used"].([]any)); got == 0 {
		t.Fatal("self_model_claim_ids_used is empty after claim confirmation")
	}
}

func completeSetup(t *testing.T, router http.Handler) {
	t.Helper()

	sessionID := startSetup(t, router)
	answerSetup(t, router, sessionID, "display_name", map[string]any{"text": "Owen"})
	answerSetup(t, router, sessionID, "focus_areas", map[string]any{"values": []string{"career", "faith"}})
	answerSetup(t, router, sessionID, "current_context", map[string]any{
		"text": "I am deciding how to balance stability and creative work.",
	})
	answerSetup(t, router, sessionID, "conversation_style", map[string]any{"value": "gentle_structured"})
	answerSetup(t, router, sessionID, "values_and_boundaries", map[string]any{
		"values":              []string{"integrity", "creative autonomy"},
		"sensitive_topics_ok": []string{"faith"},
		"avoid_topics":        []string{"diet talk"},
	})
	answerSetup(t, router, sessionID, "memory_consent", map[string]any{
		"memory_opt_in":     true,
		"review_before_use": false,
	})

	complete := doJSON(t, router, http.MethodPost, "/v1/me/setup/sessions/"+sessionID+"/complete", map[string]any{
		"memory_opt_in":     true,
		"review_before_use": false,
	})
	if complete.Code != http.StatusOK {
		t.Fatalf("complete setup status = %d, want %d; body=%s", complete.Code, http.StatusOK, complete.Body.String())
	}
}

func createConversation(t *testing.T, router http.Handler) string {
	t.Helper()

	create := doJSON(t, router, http.MethodPost, "/v1/conversations", map[string]any{
		"mode":  "reflection",
		"title": "Career stability vs creative autonomy",
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("create conversation status = %d, want %d; body=%s", create.Code, http.StatusCreated, create.Body.String())
	}
	var createBody map[string]any
	decodeJSON(t, create.Body, &createBody)
	return createBody["conversation_id"].(string)
}

func startSetup(t *testing.T, router http.Handler) string {
	t.Helper()

	rec := doJSON(t, router, http.MethodPost, "/v1/me/setup/sessions", map[string]any{
		"client_context": map[string]any{
			"locale":      "ko-KR",
			"timezone":    "Asia/Seoul",
			"platform":    "ios",
			"app_version": "0.1.0",
		},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("start setup status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var body map[string]any
	decodeJSON(t, rec.Body, &body)
	if body["current_step"].(map[string]any)["step_id"] != "display_name" {
		t.Fatalf("current_step.step_id = %v, want display_name", body["current_step"].(map[string]any)["step_id"])
	}
	return body["setup_session_id"].(string)
}

func answerSetup(t *testing.T, router http.Handler, sessionID, stepID string, answer map[string]any) {
	t.Helper()

	rec := doJSON(t, router, http.MethodPost, "/v1/me/setup/sessions/"+sessionID+"/answers", map[string]any{
		"step_id": stepID,
		"answer":  answer,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("answer %s status = %d, want %d; body=%s", stepID, rec.Code, http.StatusOK, rec.Body.String())
	}
}

func doJSON(t *testing.T, router http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		reader = bytes.NewReader(raw)
	}

	req := httptest.NewRequest(method, path, reader)
	req.AddCookie(&http.Cookie{Name: "hunch_session", Value: "test-session"})
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func decodeJSON(t *testing.T, reader io.Reader, out any) {
	t.Helper()

	if err := json.NewDecoder(reader).Decode(out); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
}
