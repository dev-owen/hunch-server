# 최초 나 설정 및 대화 경험 API 해설

작성일: 2026-05-29
상태: Draft
대상 레포: `hunch-server`
상세 스펙: [2026-05-29-initial-self-setup-conversation-api.md](./2026-05-29-initial-self-setup-conversation-api.md)

## 문서 목적

이 문서는 Hunch 사용자가 로그인한 직후 처음 마주하는 "나를 설정하는 경험"과 이후 "개인화된 성찰 대화"를 서버 API 관점에서 설명한다.

상세 API 계약은 원문 스펙 문서에 정의되어 있고, 이 문서는 그 스펙을 제품 경험, 서버 책임, 클라이언트 흐름, 구현 순서 중심으로 풀어쓴 해설 문서다.

핵심 목표는 세 가지다.

1. 사용자가 Hunch에게 자신을 처음 소개할 수 있는 짧고 자연스러운 설정 흐름을 제공한다.
2. 설정에서 얻은 정보와 이후 대화를 연결해 "나를 이해한다"는 개인화 체감을 만든다.
3. Hunch가 사용자를 단정하지 않도록 모든 장기 기억을 근거 기반 claim으로 관리한다.

## 전제

이 API는 인증 이후의 경험을 다룬다. 계정 생성, 로그인, 로그아웃, 계정 삭제는 별도 인증 스펙의 범위다.

현재 스펙은 다음 전제를 둔다.

- 사용자는 이미 로그인되어 있다.
- 서버는 `hunch_session` HttpOnly cookie를 통해 세션을 확인한다.
- 모든 API는 기본적으로 `/v1` 경로 아래에 둔다.
- 클라이언트는 로그인 직후 `GET /v1/me/bootstrap`을 호출해 다음 화면을 결정한다.
- 사용자의 설정 답변과 대화 내용은 장기 기억으로 바로 확정되지 않고, evidence와 claim으로 분리해 저장한다.

## 전체 경험 요약

로그인 이후 Hunch의 첫 경험은 크게 네 단계다.

1. 앱이 사용자 상태를 조회한다.
2. 최초 설정이 필요하면 setup session을 시작하거나 이어서 진행한다.
3. 설정이 끝나면 profile과 초기 self model 후보 claim을 만든다.
4. 사용자는 conversation을 시작하고, Hunch는 설정 정보와 self model을 참고해 구조화된 답변을 생성한다.

```text
로그인 완료
  |
  v
GET /v1/me/bootstrap
  |
  +-- setup 필요
  |     |
  |     v
  |   POST /v1/me/setup/sessions
  |     |
  |     v
  |   POST /v1/me/setup/sessions/{id}/answers
  |     |
  |     v
  |   POST /v1/me/setup/sessions/{id}/complete
  |
  +-- setup 완료 또는 skip
        |
        v
      POST /v1/conversations
        |
        v
      POST /v1/conversations/{id}/messages
```

## 설계의 핵심 관점

### 1. 최초 설정은 form이 아니라 짧은 대화에 가깝다

Hunch의 첫 설정은 단순 프로필 입력이 아니다. 사용자가 "요즘 어떤 고민이 있는지", "어떤 대화 방식이 편한지", "어떤 가치나 경계가 중요한지"를 가볍게 알려주는 흐름이다.

그래서 API도 고정된 profile update 하나로 끝내지 않고, `setup_session`과 `setup_answer`를 분리한다. 이렇게 하면 클라이언트는 단계별 질문 UI를 만들 수 있고, 서버는 중간 이탈 후 이어하기를 지원할 수 있다.

### 2. 사용자가 직접 말한 것과 모델이 추론한 것을 분리한다

Hunch에서 가장 위험한 실패는 사용자를 너무 빨리 단정하는 것이다.

예를 들어 사용자가 "요즘 커리어가 불안하다"고 말했을 때, 서버가 곧바로 "이 사용자는 불안형이다" 같은 장기 기억을 만들면 안 된다. 대신 원문 답변은 evidence로 저장하고, 모델이 만든 해석은 `model_inferred` claim으로 별도 관리해야 한다.

claim은 다음 정보를 가진다.

- 어떤 필드인지: `core_desires`, `constraints`, `values` 등
- 어떤 근거에서 나왔는지: `evidence_ids`
- 누가 만든 정보인지: `user_declared`, `model_inferred`
- 얼마나 확실한지: `confidence`
- 사용자가 검토했는지: `review_state`

### 3. self model은 사용자가 고칠 수 있어야 한다

Hunch가 사용자를 오래 이해하려면, 기억이 쌓이는 것만큼 수정 가능성이 중요하다.

따라서 `PATCH /v1/me/self-model/claims/{claim_id}`는 필수 API다. 사용자는 claim을 확인, 수정, 거절, 숨김 처리할 수 있어야 한다.

이 API가 있어야 Hunch의 개인화가 "몰래 쌓이는 프로파일링"이 아니라 "사용자와 함께 다듬는 자기 이해"가 된다.

### 4. 대화 응답은 구조화된 성찰 형식을 따른다

`reflection` 모드의 assistant message는 자유 텍스트 하나가 아니라 `hunch_reflection_v0` 형식을 따른다.

필수 섹션은 다음 다섯 가지다.

- `What You Want`
- `Current Reality`
- `Hidden Tension`
- `Reframing`
- `Next Small Step (24-72h)`

이 구조는 Hunch의 제품 정체성과 연결된다. 답변은 위로나 조언만 제공하는 것이 아니라, 이상과 현실 사이의 긴장을 정리하고 다음 작은 행동으로 이어져야 한다.

## 주요 리소스 설명

### User Bootstrap

`GET /v1/me/bootstrap`은 로그인 직후 클라이언트가 가장 먼저 호출하는 API다.

이 API는 클라이언트에게 다음 질문에 대한 답을 준다.

- 이 사용자는 최초 설정이 필요한가?
- 진행 중인 setup session이 있는가?
- 이미 생성된 self model snapshot이 있는가?
- 최근 conversation이 있는가?
- 다음 화면은 setup, memory review, conversation 중 무엇인가?

이 API가 있으면 클라이언트는 여러 endpoint를 따로 호출하지 않고 첫 화면 라우팅을 결정할 수 있다.

### Setup Session

setup session은 최초 설정의 상태 머신이다.

주요 상태는 다음과 같다.

- `not_started`: 아직 설정을 시작하지 않음
- `in_progress`: 설정 진행 중
- `completed`: 필수 질문을 모두 답하고 완료함
- `skipped`: 사용자가 설정을 나중으로 미룸

setup session이 필요한 이유는 다음과 같다.

- 사용자가 중간에 앱을 닫아도 이어서 진행할 수 있다.
- 단계별 validation을 서버에서 일관되게 처리할 수 있다.
- setup answer를 raw evidence로 남길 수 있다.
- 완료 시점에 profile 생성과 self model bootstrap을 한 번에 처리할 수 있다.

### Profile

profile은 사용자가 직접 수정 가능한 선호 설정이다.

예시는 다음과 같다.

- 표시 이름
- 언어와 타임존
- 대화 깊이
- 직접적인 톤을 선호하는지
- 기본 답변 언어
- 집중하고 싶은 주제
- 피하고 싶은 주제
- memory opt-in 여부

profile은 self model과 다르다. profile은 사용자가 설정한 preference이고, self model은 evidence 기반으로 쌓이는 장기 맥락이다.

### Self Model Claim

self model claim은 Hunch가 사용자에 대해 알고 있다고 판단하는 하나의 기억 단위다.

claim 예시는 다음과 같다.

```json
{
  "field": "core_desires",
  "label": "Wants more creative autonomy in work",
  "source": "user_declared",
  "evidence_ids": ["ev_01J..."],
  "confidence": 0.82,
  "stability": "candidate",
  "review_state": "unreviewed"
}
```

claim은 반드시 evidence를 가져야 한다. evidence 없이 "사용자는 이런 사람이다"라고 저장하면 안 된다.

self model에서 다루는 주요 필드는 다음과 같다.

- `core_desires`: 사용자가 바라는 방향
- `constraints`: 현실적 제약
- `recurring_conflicts`: 반복되는 갈등 축
- `emotional_patterns`: 감정 패턴
- `values`: 중요하게 여기는 가치
- `growth_signals`: 긍정적 변화 신호
- `temporary_state`: 일시적 상태
- `identity_statements`: 정체성 관련 자기 진술
- `open_questions`: 앞으로 더 물어볼 질문

### Conversation

conversation은 사용자의 실제 대화 경험이다.

conversation에는 여러 mode가 있다.

- `onboarding`: 설정과 이어지는 가벼운 첫 대화
- `reflection`: 이상과 현실 사이의 고민을 구조화하는 대화
- `check_in`: 현재 상태를 짧게 업데이트하는 대화
- `memory_review`: Hunch가 기억한 내용을 사용자와 검토하는 대화

v0에서 가장 중요한 mode는 `reflection`이다. 이 모드는 Hunch의 기본 응답 계약인 `hunch_reflection_v0`을 따른다.

### Memory Update Preview

대화 응답 이후 서버는 장기 기억 업데이트 후보를 만들 수 있다.

하지만 이 후보는 곧바로 안정적인 기억이 아니다. 응답 payload의 `memory_update_preview`는 클라이언트가 "이번 대화에서 Hunch가 어떤 후보 기억을 만들었는지" 보여줄 수 있게 한다.

상태는 다음 중 하나다.

- `queued`: memory update job이 예약됨
- `skipped`: 이번 메시지는 기억 업데이트 대상이 아님
- `disabled`: 사용자가 memory opt-in을 끔

## 엔드포인트 요약

| Method | Path | 역할 |
| --- | --- | --- |
| `GET` | `/v1/me/bootstrap` | 로그인 직후 사용자 상태와 다음 화면 결정 |
| `POST` | `/v1/me/setup/sessions` | 최초 설정 세션 생성 또는 재개 |
| `GET` | `/v1/me/setup/sessions/{setup_session_id}` | 설정 진행 상태와 현재 질문 조회 |
| `POST` | `/v1/me/setup/sessions/{setup_session_id}/answers` | 설정 질문 하나에 대한 답변 저장 |
| `POST` | `/v1/me/setup/sessions/{setup_session_id}/skip` | 설정을 건너뛰고 가벼운 경험으로 진입 |
| `POST` | `/v1/me/setup/sessions/{setup_session_id}/complete` | 설정 완료, profile 저장, 초기 claim 생성 |
| `GET` | `/v1/me/profile` | 사용자 profile preference 조회 |
| `PATCH` | `/v1/me/profile` | 사용자 profile preference 수정 |
| `GET` | `/v1/me/self-model` | 사용자에게 보여줄 수 있는 self model 조회 |
| `PATCH` | `/v1/me/self-model/claims/{claim_id}` | self model claim 확인, 수정, 거절, 숨김 |
| `POST` | `/v1/conversations` | 새 대화 생성 |
| `GET` | `/v1/conversations` | 대화 목록 조회 |
| `GET` | `/v1/conversations/{conversation_id}` | 대화 상세와 메시지 조회 |
| `POST` | `/v1/conversations/{conversation_id}/messages` | 사용자 메시지 전송 및 assistant 응답 생성 |
| `POST` | `/v1/conversations/{conversation_id}/messages/stream` | SSE 기반 assistant 응답 스트리밍 |
| `POST` | `/v1/conversations/{conversation_id}/feedback` | assistant 응답에 대한 사용자 피드백 저장 |

## 최초 설정 단계

v0에서 권장하는 setup step은 여섯 개다.

### 1. `display_name`

Hunch가 사용자를 어떻게 부를지 정한다.

서버는 1자 이상 40자 이하 정도의 validation을 둔다.

### 2. `focus_areas`

사용자가 Hunch에게 기대하는 주제를 고른다.

예시는 다음과 같다.

- 커리어
- 관계
- 자기 이해
- 신앙과 가치관

이 값은 첫 대화의 starter prompt와 response framing에 영향을 줄 수 있다.

### 3. `current_context`

현재 가장 신경 쓰이는 상황을 한 문단으로 받는다.

이 단계가 가장 중요하다. 첫 대화가 개인화되어 느껴지려면 최소한 하나의 고신호 context가 필요하다.

단, 이 답변은 곧바로 장기 성격 추론으로 저장하지 않고 setup answer evidence로 먼저 저장한다.

### 4. `conversation_style`

사용자가 선호하는 대화 방식을 고른다.

예시는 다음과 같다.

- 차분하게 정리해주기
- 명확하고 실행 중심
- 깊게 질문하며 탐색

이 값은 assistant tone과 답변 밀도에 영향을 준다.

### 5. `values_and_boundaries`

사용자가 중요하게 여기는 가치와 피하고 싶은 주제를 받는다.

이 단계는 두 가지 목적이 있다.

- Hunch가 사용자의 가치에 맞는 방향으로 답변하도록 돕는다.
- 사용자가 불편한 주제를 무리하게 개인화에 사용하지 않도록 막는다.

### 6. `memory_consent`

Hunch가 setup answer와 conversation history를 장기 기억 업데이트에 사용해도 되는지 확인한다.

중요한 분기:

- `memory_opt_in = true`: 대화 이후 memory update job을 만들 수 있다.
- `memory_opt_in = false`: profile preference는 저장하되 self model claim은 만들지 않는다.
- `review_before_use = true`: claim을 만들더라도 사용자가 확인하기 전에는 답변 개인화에 쓰지 않는다.

## 클라이언트 구현 관점

클라이언트는 로그인 직후 항상 bootstrap을 호출한다.

bootstrap 결과에 따라 화면은 다음처럼 분기한다.

- `setup.required = true`, active session 없음: setup 시작 화면
- `setup.required = true`, active session 있음: setup 이어하기 화면
- `setup.next_step = review_memory`: 기억 검토 화면
- `setup.required = false`: conversation home 또는 최근 대화 화면

setup 화면은 서버가 내려주는 `current_step`을 렌더링하는 방식이 좋다. 이렇게 하면 질문 순서와 validation을 서버가 통제할 수 있고, 클라이언트는 step type별 컴포넌트만 관리하면 된다.

지원해야 하는 step type 예시는 다음과 같다.

- `short_text`
- `long_text`
- `multi_select`
- `choice_group`
- `toggle`

conversation 화면은 non-streaming API를 먼저 붙이고, 응답 품질과 저장 흐름이 안정된 뒤 SSE streaming을 붙이는 순서가 좋다.

## 서버 구현 관점

서버는 이 기능을 크게 다섯 영역으로 나눠 구현할 수 있다.

### 1. Profile and Setup

필요한 책임:

- 사용자 profile 생성과 조회
- setup session 생성, 재개, 완료, skip
- setup answer 저장
- step validation
- setup 완료 시 profile preference 반영

### 2. Evidence

필요한 책임:

- setup answer를 raw evidence로 저장
- conversation message를 evidence로 참조 가능하게 저장
- claim이 어떤 evidence에서 나왔는지 추적

### 3. Self Model

필요한 책임:

- 현재 self model snapshot 조회
- claim 생성과 상태 관리
- claim review action 처리
- user-declared claim과 model-inferred claim 구분
- hidden/rejected claim을 개인화 context에서 제외

### 4. Conversation

필요한 책임:

- conversation 생성, 목록, 상세 조회
- user message 저장
- assistant response 생성
- response contract validation
- feedback 저장

### 5. Memory Update Worker

필요한 책임:

- conversation 이후 memory update job enqueue
- 후보 claim 추출
- promotion rule 적용
- snapshot 갱신 또는 projection 갱신
- claim event audit log 저장

## 저장 모델 방향

정확한 테이블명은 구현 과정에서 조정할 수 있지만, 최소한 다음 개념은 분리하는 것이 좋다.

- `user_profiles`: 사용자 profile preference
- `setup_sessions`: 최초 설정 진행 상태
- `setup_answers`: 사용자가 직접 작성한 setup 답변
- `evidence_refs`: setup answer, message, document chunk 등 근거 참조
- `self_model_snapshots`: 특정 시점의 self model
- `self_model_claims`: 장기 기억 후보와 확정 claim
- `self_model_claim_events`: claim 생성, 수정, 확인, 거절, 숨김 이력
- `conversations`: 대화방 단위 metadata
- `conversation_messages`: user, assistant message
- `conversation_feedback`: 응답 품질 피드백

기존 `documents`, `document_chunks`는 긴 메모나 외부 글을 저장하는 데 사용할 수 있다. setup answer와 conversation message는 제품 상태와 직접 연결되므로 별도 first-class table로 두는 편이 안전하다.

## 기억 업데이트 정책

Hunch의 장기 기억은 보수적으로 업데이트해야 한다.

setup 완료 시:

- 답변 원문을 evidence로 저장한다.
- 사용자가 명시한 내용만 `user_declared` claim 후보로 만든다.
- 진단적 표현이나 성격 단정은 만들지 않는다.
- identity 관련 claim은 사용자가 확인하기 전까지 조심스럽게 다룬다.

conversation 이후:

- `memory_opt_in = true`일 때만 memory update job을 만든다.
- 최근 message window에서 후보 claim을 추출한다.
- 한 번 나온 정보는 stable memory로 승격하지 않는다.
- 반복 관찰, 시간 간격, confidence 기준을 만족할 때만 승격한다.
- 일시적 감정이나 피로 상태는 `temporary_state`로 둔다.

권장 promotion 기준:

- 독립적인 근거가 2개 이상 있어야 한다.
- 근거 사이에 최소 7일 이상 간격이 있어야 한다.
- confidence가 `0.60` 이상이어야 한다.

## 안전과 프라이버시

이 API의 안전 원칙은 "개인화하되 단정하지 않는다"이다.

서버와 assistant는 다음을 지켜야 한다.

- 모델 추론을 사실처럼 말하지 않는다.
- "항상", "절대" 같은 강한 일반화를 피한다.
- 자기비난, 절망, 극단적 결론을 강화하지 않는다.
- 사실과 해석을 분리한다.
- reflection mode에서는 24-72시간 안에 할 수 있는 작은 다음 행동을 제안한다.
- 민감하거나 위험한 요청에는 안전 우선 응답을 반환한다.
- raw setup answer, raw chat message, full prompt를 기본 로그에 남기지 않는다.

## 에러 처리

모든 에러는 공통 shape을 따른다.

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

주요 에러 코드는 다음과 같다.

- `unauthenticated`: 세션 없음 또는 만료
- `forbidden`: 리소스 접근 권한 없음
- `invalid_request`: 요청 형식 또는 validation 실패
- `not_found`: 리소스 없음
- `conflict`: version 충돌 또는 중복 완료
- `rate_limited`: 요청 과다
- `safety_blocked`: 안전 정책상 직접 응답 불가
- `internal`: 서버 내부 오류

## 구현 우선순위

추천 구현 순서는 다음과 같다.

1. profile과 setup session persistence를 만든다.
2. `GET /v1/me/bootstrap`을 구현한다.
3. setup session 생성, answer 저장, complete, skip을 구현한다.
4. profile 조회와 수정을 구현한다.
5. self model claim 조회와 사용자 correction API를 구현한다.
6. conversation 생성, 목록, 상세 조회를 구현한다.
7. non-streaming message 생성 API를 먼저 구현한다.
8. response contract validation을 추가한다.
9. memory update job enqueue와 preview를 붙인다.
10. SSE streaming을 마지막에 추가한다.
11. feedback API와 observability metric을 추가한다.

## v0에서 꼭 지켜야 할 범위

v0는 "완벽한 개인 AI"를 만드는 단계가 아니다. 첫 목표는 사용자가 첫 대화에서 "내 맥락을 어느 정도 이해하고 있네"라고 느끼는 것이다.

그래서 v0에서 꼭 필요한 것은 다음이다.

- 로그인 직후 올바른 화면으로 보내는 bootstrap API
- 짧지만 의미 있는 최초 설정
- setup answer를 evidence로 저장하는 구조
- 사용자가 고칠 수 있는 self model claim
- `hunch_reflection_v0`에 맞는 구조화된 assistant 응답
- 기억 업데이트를 즉시 확정하지 않는 보수적 정책

반대로 v0에서 미뤄도 되는 것은 다음이다.

- 모든 setup 질문을 LLM이 동적으로 생성하는 기능
- 복잡한 외부 데이터 import
- 고급 memory graph
- 실시간 streaming 우선 구현
- 모든 claim field에 대한 세밀한 review UX

## 남은 결정 사항

구현 전에 추가로 정해야 할 질문은 다음과 같다.

- setup step은 서버의 deterministic catalog로 고정할 것인가, conversation orchestrator가 일부 생성하게 할 것인가?
- self model snapshot은 매 업데이트마다 새로 만들 것인가, event-sourced projection으로 관리할 것인가?
- `review_before_use`는 전체 설정으로 둘 것인가, claim field별 설정으로 나눌 것인가?
- 첫 setup에서 기존 메모 import를 허용할 것인가, 첫 대화 이후로 미룰 것인가?
- 위험 상황이나 위기 대응 문구는 어떤 기준과 지역 리소스를 따를 것인가?

## 한 줄 요약

이 스펙은 Hunch의 첫 로그인 이후 경험을 "설정, 기억, 대화"로 분리한다. 사용자는 자신을 가볍게 소개하고, 서버는 그 정보를 근거 기반 기억으로 조심스럽게 관리하며, 대화 API는 그 기억을 바탕으로 이상과 현실 사이의 고민을 구조화한다.
