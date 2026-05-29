# Initial Self Setup And Conversation API Spec

Created: 2026-05-29
Status: Draft
Target repo: `hunch-server`

## Goal

Design the authenticated API surface for the first Hunch experience after login:

1. Help the user set up "me" with enough context for Hunch to feel personal without over-claiming.
2. Start a reflective conversation that uses the initial setup, recent messages, and long-term self model safely.
3. Keep every memory claim evidence-backed, reviewable, and correctable by the user.

This spec starts after account authentication has succeeded. It assumes the auth plan's opaque `hunch_session` HttpOnly cookie is present.

## Product Principles

- The first setup should feel like a short conversation, not a form.
- Hunch should ask for only the minimum context needed to produce a useful first answer.
- User-authored statements are stronger than model inference.
- Inferred self model claims must stay tentative until enough evidence exists.
- The user can inspect, correct, reject, or hide memory claims.
- Conversation output must separate facts from interpretation and avoid reinforcing self-blame.

## API Conventions

- Base path: `/v1`
- Auth: all endpoints require `hunch_session` unless explicitly marked public.
- Content type: `application/json; charset=utf-8`
- Timestamps: ISO-8601 UTC strings.
- IDs: UUID strings unless otherwise noted.
- Pagination: cursor-based, using `limit` and `cursor`.
- Idempotency: mutating endpoints that may be retried accept `Idempotency-Key`.
- Localization: clients may send `Accept-Language`; server stores `locale` on profile preferences.

## Common Error Shape

```json
{
  "error": {
    "code": "invalid_request",
    "message": "display name must be 1-40 characters",
    "details": {
      "field": "display_name"
    },
    "request_id": "req_01J..."
  }
}
```

Common codes:

- `unauthenticated`: missing or invalid session.
- `forbidden`: session is valid but account cannot access the resource.
- `invalid_request`: malformed JSON, invalid enum, or validation failure.
- `not_found`: resource does not exist for this account.
- `conflict`: stale version, duplicate completion, or conflicting state transition.
- `rate_limited`: too many setup or chat requests.
- `safety_blocked`: request cannot be answered directly because it violates safety rules.
- `internal`: unexpected server error.

## Experience Flow

```text
[Signed in]
    |
    v
GET /v1/me/bootstrap
    |
    +--> setup_required = true
    |       |
    |       v
    |  POST /v1/me/setup/sessions
    |       |
    |       v
    |  POST /v1/me/setup/sessions/{setup_session_id}/answers
    |       |
    |       v
    |  POST /v1/me/setup/sessions/{setup_session_id}/complete
    |
    +--> setup_required = false
            |
            v
POST /v1/conversations
    |
    v
POST /v1/conversations/{conversation_id}/messages
    |
    +--> assistant response
    +--> memory update preview
    +--> suggested next prompts
```

## Resource Model

### User Bootstrap

The client calls bootstrap immediately after login to decide whether to show setup, resume setup, or open the main conversation surface.

```json
{
  "account_id": "acc_01J...",
  "profile": {
    "user_id": "usr_01J...",
    "display_name": "Owen",
    "locale": "ko-KR",
    "timezone": "Asia/Seoul",
    "setup_status": "completed",
    "setup_completed_at": "2026-05-29T09:30:00Z"
  },
  "setup": {
    "required": false,
    "active_setup_session_id": null,
    "next_step": null,
    "completion_ratio": 1
  },
  "self_model": {
    "snapshot_id": "sms_01J...",
    "claim_count": 12,
    "last_updated_at": "2026-05-29T09:31:00Z",
    "needs_review_count": 2
  },
  "conversation": {
    "suggested_start_mode": "reflection",
    "latest_conversation_id": "con_01J..."
  }
}
```

### Initial Self Setup Profile

The setup profile stores user-declared setup answers separately from inferred memory.

```json
{
  "display_name": "Owen",
  "locale": "ko-KR",
  "timezone": "Asia/Seoul",
  "conversation_style": {
    "depth": "balanced",
    "directness": "gentle",
    "default_language": "ko",
    "reflection_format": "structured"
  },
  "focus_areas": ["career", "relationships", "faith"],
  "current_context": {
    "one_line": "I am deciding how to balance stability and creative work.",
    "season": "career transition",
    "important_constraints": ["monthly income", "limited energy on weekdays"]
  },
  "values": [
    {
      "label": "integrity",
      "source": "user_declared"
    },
    {
      "label": "creative autonomy",
      "source": "user_declared"
    }
  ],
  "boundaries": {
    "avoid_topics": ["diet talk"],
    "sensitive_topics_ok": ["faith"],
    "memory_opt_in": true
  }
}
```

### Self Model Claim

Self model claims follow the Hunch v0 memory schema. User setup answers may seed claims, but those claims should be marked as `user_declared` and still include evidence references.

```json
{
  "claim_id": "clm_01J...",
  "field": "core_desires",
  "label": "Wants more creative autonomy in work",
  "source": "user_declared",
  "evidence_ids": ["ev_01J..."],
  "source_types": ["setup_answer"],
  "confidence": 0.82,
  "first_seen": "2026-05-29",
  "last_seen": "2026-05-29",
  "stability": "candidate",
  "status": "active",
  "review_state": "unreviewed"
}
```

Allowed `field` values:

- `core_desires`
- `constraints`
- `recurring_conflicts`
- `emotional_patterns`
- `values`
- `growth_signals`
- `temporary_state`
- `identity_statements`
- `open_questions`

Allowed `source` values:

- `user_declared`
- `model_inferred`
- `system_imported`

Allowed `review_state` values:

- `unreviewed`
- `confirmed`
- `corrected`
- `rejected`
- `hidden`

### Conversation Message

```json
{
  "message_id": "msg_01J...",
  "conversation_id": "con_01J...",
  "role": "assistant",
  "content": {
    "format": "hunch_reflection_v0",
    "markdown": "## What You Want\n- ...",
    "sections": [
      {
        "type": "what_you_want",
        "title": "What You Want",
        "items": ["You want more creative control without losing stability."]
      }
    ]
  },
  "metadata": {
    "evidence_ids_used": ["ev_01J..."],
    "self_model_claim_ids_used": ["clm_01J..."],
    "risk_flags": {
      "negativity_reinforcement": false,
      "value_overreach": false,
      "false_personalization": false
    }
  },
  "created_at": "2026-05-29T09:45:00Z"
}
```

## Endpoints

### GET `/v1/me/bootstrap`

Return the authenticated user's post-login state.

Use this endpoint after signin and app launch.

Response `200`:

```json
{
  "account_id": "acc_01J...",
  "profile": {
    "user_id": "usr_01J...",
    "display_name": null,
    "locale": "ko-KR",
    "timezone": "Asia/Seoul",
    "setup_status": "not_started",
    "setup_completed_at": null
  },
  "setup": {
    "required": true,
    "active_setup_session_id": null,
    "next_step": "create_setup_session",
    "completion_ratio": 0
  },
  "self_model": {
    "snapshot_id": null,
    "claim_count": 0,
    "last_updated_at": null,
    "needs_review_count": 0
  },
  "conversation": {
    "suggested_start_mode": "onboarding",
    "latest_conversation_id": null
  }
}
```

State rules:

- `setup.required = true` when the profile has no completed setup and no skipped setup.
- `active_setup_session_id` is returned when the user left setup mid-flow.
- `setup.next_step` is one of `create_setup_session`, `continue_setup`, `review_memory`, `start_conversation`, or `none`.

### POST `/v1/me/setup/sessions`

Create or resume the first setup session.

Request:

```json
{
  "client_context": {
    "locale": "ko-KR",
    "timezone": "Asia/Seoul",
    "platform": "ios",
    "app_version": "0.1.0"
  }
}
```

Response `201`:

```json
{
  "setup_session_id": "set_01J...",
  "status": "in_progress",
  "current_step": {
    "step_id": "display_name",
    "type": "short_text",
    "prompt": "Hunch가 당신을 어떻게 부르면 좋을까요?",
    "required": true,
    "constraints": {
      "min_length": 1,
      "max_length": 40
    }
  },
  "progress": {
    "completed_steps": 0,
    "total_steps": 6,
    "completion_ratio": 0
  }
}
```

If an active setup session exists, return `200` with the existing session instead of creating a duplicate.

### GET `/v1/me/setup/sessions/{setup_session_id}`

Return setup session state and all answered steps.

Response `200`:

```json
{
  "setup_session_id": "set_01J...",
  "status": "in_progress",
  "answers": [
    {
      "step_id": "display_name",
      "answer": {
        "text": "Owen"
      },
      "answered_at": "2026-05-29T09:21:00Z"
    }
  ],
  "current_step": {
    "step_id": "focus_areas",
    "type": "multi_select",
    "prompt": "요즘 Hunch가 가장 잘 도와줬으면 하는 주제는 무엇인가요?",
    "required": true,
    "options": [
      {
        "value": "career",
        "label": "일과 커리어"
      },
      {
        "value": "relationships",
        "label": "관계"
      },
      {
        "value": "self_understanding",
        "label": "나에 대한 이해"
      },
      {
        "value": "faith",
        "label": "신앙과 가치관"
      }
    ],
    "constraints": {
      "min_items": 1,
      "max_items": 4
    }
  },
  "progress": {
    "completed_steps": 1,
    "total_steps": 6,
    "completion_ratio": 0.17
  }
}
```

### POST `/v1/me/setup/sessions/{setup_session_id}/answers`

Save one setup answer and return the next step.

Request:

```json
{
  "step_id": "current_context",
  "answer": {
    "text": "I keep wondering whether I should choose a stable job or keep building creative products."
  }
}
```

Response `200`:

```json
{
  "setup_session_id": "set_01J...",
  "saved_answer": {
    "step_id": "current_context",
    "answer_id": "ans_01J...",
    "answered_at": "2026-05-29T09:24:00Z"
  },
  "current_step": {
    "step_id": "conversation_style",
    "type": "choice_group",
    "prompt": "Hunch의 대화 방식은 어느 쪽이 편한가요?",
    "required": true,
    "options": [
      {
        "value": "gentle_structured",
        "label": "차분하게 정리해주기"
      },
      {
        "value": "direct_practical",
        "label": "명확하고 실행 중심"
      },
      {
        "value": "deep_reflective",
        "label": "깊게 질문하며 탐색"
      }
    ]
  },
  "progress": {
    "completed_steps": 4,
    "total_steps": 6,
    "completion_ratio": 0.67
  }
}
```

Validation rules:

- The `step_id` must match the current step unless the session allows backfill.
- Answers should be stored as raw user evidence before any model extraction.
- Text answers over 2,000 characters return `invalid_request`.
- The server should allow answer replacement while setup is `in_progress`.

### POST `/v1/me/setup/sessions/{setup_session_id}/skip`

Allow the user to skip non-required setup and start with a lightweight experience.

Request:

```json
{
  "reason": "user_preferred_later"
}
```

Response `200`:

```json
{
  "setup_session_id": "set_01J...",
  "status": "skipped",
  "profile": {
    "setup_status": "skipped",
    "setup_completed_at": null
  },
  "next_action": {
    "type": "start_conversation",
    "suggested_mode": "light_reflection"
  }
}
```

Skipping should not create long-term self model claims.

### POST `/v1/me/setup/sessions/{setup_session_id}/complete`

Finalize setup, create profile preferences, store setup evidence, and enqueue self model bootstrap.

Request:

```json
{
  "memory_opt_in": true,
  "review_before_use": false
}
```

Response `200`:

```json
{
  "setup_session_id": "set_01J...",
  "status": "completed",
  "profile": {
    "user_id": "usr_01J...",
    "display_name": "Owen",
    "locale": "ko-KR",
    "timezone": "Asia/Seoul",
    "setup_status": "completed",
    "setup_completed_at": "2026-05-29T09:30:00Z"
  },
  "self_model": {
    "snapshot_id": "sms_01J...",
    "candidate_claims": [
      {
        "claim_id": "clm_01J...",
        "field": "core_desires",
        "label": "Wants more creative autonomy in work",
        "source": "user_declared",
        "confidence": 0.82,
        "review_state": "unreviewed"
      }
    ],
    "needs_review_count": 2
  },
  "next_action": {
    "type": "start_conversation",
    "suggested_mode": "reflection",
    "starter_prompt": "지금 가장 정리하고 싶은 고민 하나를 말해줘."
  }
}
```

State rules:

- Completion is allowed only when all required steps have valid answers.
- Calling complete twice returns `200` with the completed session and does not duplicate claims.
- If `memory_opt_in = false`, setup answers can still configure profile preferences but must not create self model claims.
- If `review_before_use = true`, generated claims are stored with `review_state = unreviewed` and excluded from response personalization until confirmed.

### GET `/v1/me/profile`

Return user-editable profile preferences.

Response `200`:

```json
{
  "user_id": "usr_01J...",
  "display_name": "Owen",
  "locale": "ko-KR",
  "timezone": "Asia/Seoul",
  "conversation_style": {
    "depth": "balanced",
    "directness": "gentle",
    "default_language": "ko",
    "reflection_format": "structured"
  },
  "focus_areas": ["career", "faith"],
  "boundaries": {
    "avoid_topics": [],
    "sensitive_topics_ok": ["faith"],
    "memory_opt_in": true
  },
  "version": 3,
  "updated_at": "2026-05-29T09:30:00Z"
}
```

### PATCH `/v1/me/profile`

Update profile preferences. This endpoint does not directly rewrite self model claims.

Request:

```json
{
  "display_name": "Owen",
  "conversation_style": {
    "depth": "deep",
    "directness": "gentle"
  },
  "version": 3
}
```

Response `200`:

```json
{
  "user_id": "usr_01J...",
  "display_name": "Owen",
  "conversation_style": {
    "depth": "deep",
    "directness": "gentle",
    "default_language": "ko",
    "reflection_format": "structured"
  },
  "version": 4,
  "updated_at": "2026-05-29T10:02:00Z"
}
```

Concurrency:

- The client should send the last seen `version`.
- If the version is stale, return `409 conflict`.

### GET `/v1/me/self-model`

Return the current user-readable self model.

Query parameters:

- `include_hidden`: default `false`
- `fields`: optional comma-separated field filter
- `review_state`: optional claim review state filter

Response `200`:

```json
{
  "snapshot_id": "sms_01J...",
  "user_id": "usr_01J...",
  "snapshot_at": "2026-05-29T09:31:00Z",
  "claims": [
    {
      "claim_id": "clm_01J...",
      "field": "core_desires",
      "label": "Wants more creative autonomy in work",
      "source": "user_declared",
      "evidence_ids": ["ev_01J..."],
      "confidence": 0.82,
      "stability": "candidate",
      "status": "active",
      "review_state": "unreviewed",
      "first_seen": "2026-05-29",
      "last_seen": "2026-05-29"
    }
  ],
  "open_questions": [
    {
      "claim_id": "clm_01K...",
      "label": "What does stability mean in this season?",
      "confidence": 0.61
    }
  ]
}
```

Privacy rule:

- The API returns user-readable labels and evidence references, not raw internal prompts or hidden model traces.

### PATCH `/v1/me/self-model/claims/{claim_id}`

Let the user correct the model.

Request:

```json
{
  "action": "correct",
  "corrected_label": "I want creative autonomy, but not at the cost of basic stability.",
  "note": "This is more precise."
}
```

Allowed actions:

- `confirm`: mark as reviewed and usable.
- `correct`: replace label with user-authored version and lower/adjust confidence according to implementation policy.
- `reject`: retire the claim.
- `hide`: keep internally excluded from personalization and default reads.

Response `200`:

```json
{
  "claim_id": "clm_01J...",
  "field": "core_desires",
  "label": "I want creative autonomy, but not at the cost of basic stability.",
  "status": "active",
  "review_state": "corrected",
  "updated_at": "2026-05-29T10:08:00Z"
}
```

### POST `/v1/conversations`

Create a conversation.

Request:

```json
{
  "mode": "reflection",
  "title": "Career stability vs creative autonomy",
  "initial_user_message": {
    "text": "I keep going back and forth between taking a stable job and building my own product."
  },
  "context_policy": {
    "use_self_model": true,
    "use_recent_conversations": true,
    "use_setup_answers": true
  }
}
```

Response `201`:

```json
{
  "conversation_id": "con_01J...",
  "mode": "reflection",
  "title": "Career stability vs creative autonomy",
  "status": "active",
  "created_at": "2026-05-29T10:15:00Z",
  "messages": [
    {
      "message_id": "msg_01J...",
      "role": "user",
      "content": {
        "text": "I keep going back and forth between taking a stable job and building my own product."
      },
      "created_at": "2026-05-29T10:15:00Z"
    }
  ],
  "next_action": {
    "type": "send_message",
    "endpoint": "/v1/conversations/con_01J.../messages"
  }
}
```

Allowed `mode` values:

- `onboarding`: setup-like conversational introduction.
- `reflection`: ideal-vs-reality dilemma structure.
- `check_in`: lightweight state update.
- `memory_review`: review self model claims with the user.

### GET `/v1/conversations`

List conversations.

Query parameters:

- `limit`: default `20`, max `50`
- `cursor`: optional
- `status`: `active`, `archived`, or `all`

Response `200`:

```json
{
  "items": [
    {
      "conversation_id": "con_01J...",
      "mode": "reflection",
      "title": "Career stability vs creative autonomy",
      "status": "active",
      "last_message_at": "2026-05-29T10:20:00Z",
      "created_at": "2026-05-29T10:15:00Z"
    }
  ],
  "next_cursor": null
}
```

### GET `/v1/conversations/{conversation_id}`

Return conversation metadata and paginated messages.

Query parameters:

- `limit`: default `30`, max `100`
- `before`: optional message cursor

Response `200`:

```json
{
  "conversation_id": "con_01J...",
  "mode": "reflection",
  "title": "Career stability vs creative autonomy",
  "status": "active",
  "messages": [
    {
      "message_id": "msg_01J...",
      "role": "user",
      "content": {
        "text": "I keep going back and forth..."
      },
      "created_at": "2026-05-29T10:15:00Z"
    },
    {
      "message_id": "msg_01K...",
      "role": "assistant",
      "content": {
        "format": "hunch_reflection_v0",
        "markdown": "## What You Want\n- ...",
        "sections": []
      },
      "metadata": {
        "evidence_ids_used": ["ev_01J..."],
        "self_model_claim_ids_used": ["clm_01J..."],
        "risk_flags": {
          "negativity_reinforcement": false,
          "value_overreach": false,
          "false_personalization": false
        }
      },
      "created_at": "2026-05-29T10:16:00Z"
    }
  ],
  "next_cursor": null
}
```

### POST `/v1/conversations/{conversation_id}/messages`

Send a user message and receive an assistant response.

Request:

```json
{
  "message": {
    "text": "I think I want freedom, but I am scared I will regret not choosing money."
  },
  "response_options": {
    "format": "hunch_reflection_v0",
    "include_memory_preview": true,
    "include_suggested_replies": true
  },
  "context_policy": {
    "use_self_model": true,
    "use_recent_conversations": true,
    "max_evidence_items": 8
  }
}
```

Response `200`:

```json
{
  "conversation_id": "con_01J...",
  "user_message": {
    "message_id": "msg_01L...",
    "role": "user",
    "content": {
      "text": "I think I want freedom, but I am scared I will regret not choosing money."
    },
    "created_at": "2026-05-29T10:20:00Z"
  },
  "assistant_message": {
    "message_id": "msg_01M...",
    "role": "assistant",
    "content": {
      "format": "hunch_reflection_v0",
      "markdown": "## What You Want\n- You want room to build something that feels genuinely yours.\n\n## Current Reality\n- Money is not an abstract concern here; you named regret and stability as real constraints.\n\n## Hidden Tension\n- This is not simply freedom versus fear. It looks more like creative autonomy versus the need for a reliable floor.\n\n## Reframing\n- The question may not be 'Do I abandon stability?' but 'What minimum stability would let me experiment without panic?'\n\n## Next Small Step (24-72h)\n- Write a one-page stability floor: monthly income needed, runway needed, and the smallest product experiment you could run beside a stable option.",
      "sections": [
        {
          "type": "what_you_want",
          "title": "What You Want",
          "items": ["You want room to build something that feels genuinely yours."]
        },
        {
          "type": "current_reality",
          "title": "Current Reality",
          "items": ["Money is not an abstract concern here; you named regret and stability as real constraints."]
        },
        {
          "type": "hidden_tension",
          "title": "Hidden Tension",
          "items": ["This is not simply freedom versus fear. It looks more like creative autonomy versus the need for a reliable floor."]
        },
        {
          "type": "reframing",
          "title": "Reframing",
          "items": ["The question may not be 'Do I abandon stability?' but 'What minimum stability would let me experiment without panic?'"]
        },
        {
          "type": "next_small_step",
          "title": "Next Small Step (24-72h)",
          "items": ["Write a one-page stability floor: monthly income needed, runway needed, and the smallest product experiment you could run beside a stable option."]
        }
      ]
    },
    "metadata": {
      "evidence_ids_used": ["ev_01J...", "ev_01K..."],
      "self_model_claim_ids_used": ["clm_01J..."],
      "risk_flags": {
        "negativity_reinforcement": false,
        "value_overreach": false,
        "false_personalization": false
      }
    },
    "created_at": "2026-05-29T10:20:04Z"
  },
  "memory_update_preview": {
    "status": "queued",
    "candidate_claims": [
      {
        "field": "recurring_conflicts",
        "label": "Creative autonomy versus reliable stability",
        "source": "model_inferred",
        "confidence": 0.58,
        "stability": "candidate"
      }
    ]
  },
  "suggested_replies": [
    {
      "text": "Help me define my stability floor."
    },
    {
      "text": "Ask me a question that clarifies what freedom means here."
    }
  ]
}
```

Response requirements:

- `assistant_message.content.format = hunch_reflection_v0` must include the five required sections from the Hunch response contract.
- The assistant should cite only evidence available to the authenticated user.
- The response may ask one precise follow-up when context is insufficient.
- `memory_update_preview.status` is `queued`, `skipped`, or `disabled`.
- Candidate claims below the promotion threshold must not appear as stable traits.

### POST `/v1/conversations/{conversation_id}/messages/stream`

Stream an assistant response using Server-Sent Events.

Request shape is the same as `POST /messages`.

Response:

```text
event: message.created
data: {"message_id":"msg_01M...","role":"assistant"}

event: content.delta
data: {"text":"## What You Want\n"}

event: content.delta
data: {"text":"- You want room to build..."}

event: memory.preview
data: {"status":"queued","candidate_claims":[]}

event: message.completed
data: {"message_id":"msg_01M...","created_at":"2026-05-29T10:20:04Z"}
```

SSE event types:

- `message.created`
- `content.delta`
- `section.completed`
- `memory.preview`
- `message.completed`
- `error`

Persistence rule:

- The server should persist the final assistant message after generation completes.
- If streaming fails mid-generation, persist the user message and mark the assistant attempt as `failed` or `partial` according to implementation policy.

### POST `/v1/conversations/{conversation_id}/feedback`

Capture response quality feedback.

Request:

```json
{
  "message_id": "msg_01M...",
  "rating": "helpful",
  "tags": ["felt_personal", "actionable"],
  "comment": "The stability floor idea was useful."
}
```

Allowed `rating` values:

- `helpful`
- `not_helpful`
- `unsafe`

Allowed `tags` examples:

- `felt_personal`
- `too_generic`
- `false_personalization`
- `actionable`
- `not_actionable`
- `too_direct`
- `too_soft`
- `unsafe`

Response `201`:

```json
{
  "feedback_id": "fbk_01J...",
  "message_id": "msg_01M...",
  "created_at": "2026-05-29T10:22:00Z"
}
```

## Setup Step Catalog

The initial setup should start small. The recommended v0 sequence is:

1. `display_name`
2. `focus_areas`
3. `current_context`
4. `conversation_style`
5. `values_and_boundaries`
6. `memory_consent`

### `display_name`

Purpose: let Hunch address the user naturally.

Answer shape:

```json
{
  "text": "Owen"
}
```

### `focus_areas`

Purpose: understand the user's first product intent.

Answer shape:

```json
{
  "values": ["career", "self_understanding"]
}
```

### `current_context`

Purpose: collect one high-signal paragraph for the first response and seed candidate memories.

Answer shape:

```json
{
  "text": "I am in a career transition and keep comparing stability with creative autonomy."
}
```

### `conversation_style`

Purpose: tune the response voice.

Answer shape:

```json
{
  "value": "gentle_structured"
}
```

### `values_and_boundaries`

Purpose: allow values personalization while preventing value overreach.

Answer shape:

```json
{
  "values": ["integrity", "creative autonomy"],
  "sensitive_topics_ok": ["faith"],
  "avoid_topics": []
}
```

### `memory_consent`

Purpose: decide whether Hunch may use setup answers and conversation history to update memory.

Answer shape:

```json
{
  "memory_opt_in": true,
  "review_before_use": false
}
```

## Persistence Implications

This spec does not require exact table names, but implementation should preserve these concepts:

- `user_profiles`: one row per account-facing Hunch user profile.
- `setup_sessions`: setup state, status, current step, completion metadata.
- `setup_answers`: raw user-authored setup answers, stored as evidence.
- `self_model_snapshots`: point-in-time self model snapshots.
- `self_model_claims`: individual evidence-backed claims.
- `self_model_claim_events`: audit log for extraction, confirmation, correction, rejection, and hiding.
- `conversations`: conversation metadata and mode.
- `conversation_messages`: user and assistant messages.
- `conversation_feedback`: explicit quality feedback.
- `evidence_refs`: references to setup answers, documents, chunks, and messages used by memory or response generation.

Existing `documents` and `document_chunks` can store longer imported notes or writings. Setup answers and chat messages should have their own first-class tables because they drive product state and audit trails.

## Memory Update Policy

During setup completion:

- Store each answer as evidence.
- Create user-declared claims only from explicit user statements.
- Do not create diagnostic, clinical, or hard personality labels.
- Mark identity-like statements as `candidate` unless the user confirms them.

After each conversation:

- Enqueue a memory update job when `memory_opt_in = true`.
- Extract candidate claims from the latest message window.
- Promote claims only when they satisfy the Hunch memory rules:
  - at least two independent records
  - separated by at least seven days
  - confidence at least `0.60`
- Keep mood-heavy or recent-only signals in `temporary_state`.
- Lower confidence before retiring a claim when contradictory evidence appears.

## Safety And Tone Requirements

The conversation API must enforce these rules:

- Do not present model inference as certainty.
- Do not use "always", "never", or hard labels about the user's identity.
- Do not amplify self-blame, hopelessness, or extreme conclusions.
- Separate facts, interpretation, and suggested next action.
- Suggest one realistic action for the next 24-72 hours when using `reflection` mode.
- If the user asks for harmful action, return a safety-first response and set `risk_flags`.
- If the message indicates immediate danger, the product should show crisis support UX. The exact escalation copy should be reviewed separately before production.

## Authorization Rules

- Every resource is scoped by authenticated account.
- Clients never pass `account_id` or `user_id` for ownership decisions.
- Conversation, setup, self model, and evidence IDs must be checked against the session account.
- Soft-deleted accounts must receive `unauthenticated` or `forbidden` depending on session policy.

## Rate Limits

Recommended starting limits:

- Setup answer writes: `60/min/account`
- Conversation message writes: `20/min/account`
- Streaming conversation starts: `10/min/account`
- Self model claim edits: `60/min/account`

Rate limit responses return `429`:

```json
{
  "error": {
    "code": "rate_limited",
    "message": "too many conversation messages; try again shortly",
    "details": {
      "retry_after_seconds": 30
    },
    "request_id": "req_01J..."
  }
}
```

## Observability

Minimum metrics:

- `hunch_setup_sessions_created_total`
- `hunch_setup_sessions_completed_total`
- `hunch_setup_answer_validation_errors_total`
- `hunch_conversation_messages_created_total`
- `hunch_conversation_generation_seconds`
- `hunch_memory_update_jobs_queued_total`
- `hunch_memory_claim_user_actions_total`
- `hunch_response_risk_flags_total`

Minimum structured log fields:

- `request_id`
- `account_id_hash`
- `endpoint`
- `conversation_id`
- `setup_session_id`
- `message_id`
- `latency_ms`
- `status_code`
- `error_code`

Do not log raw setup answers, raw chat messages, or full model prompts by default.

## Implementation Milestones

1. Add profile and setup persistence.
2. Implement `/v1/me/bootstrap` and setup session endpoints.
3. Implement profile read/update.
4. Add self model claim read and user correction endpoints.
5. Implement conversation create/list/get.
6. Implement non-streaming message generation with response contract validation.
7. Add memory update job enqueue and candidate preview.
8. Add SSE streaming once the synchronous path is stable.
9. Add feedback endpoint and response quality metrics.

## Open Decisions

- Whether setup should be fully deterministic from server-provided steps or partially generated by the conversation orchestrator.
- Whether self model snapshots are rebuilt on every memory update or maintained as event-sourced projections.
- Whether `review_before_use` should be a global setting or configurable per claim field.
- Whether imported notes should be available during first setup or introduced after the first successful conversation.
- Exact crisis support copy and jurisdiction-aware resource handling.
