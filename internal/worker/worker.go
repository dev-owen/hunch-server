package worker

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wonjong/hunch-server/internal/db/dbgen"
)

type Queue struct {
	queries dbgen.Querier
}

type Job struct {
	ID          int64
	Type        string
	Payload     []byte
	Attempts    int32
	AvailableAt time.Time
}

func NewQueue(pool *pgxpool.Pool) *Queue {
	return &Queue{queries: dbgen.New(pool)}
}

func (q *Queue) Enqueue(ctx context.Context, jobType string, payload any, availableAt time.Time) (Job, error) {
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return Job{}, err
	}

	if availableAt.IsZero() {
		availableAt = time.Now().UTC()
	}

	job, err := q.queries.EnqueueJob(ctx, dbgen.EnqueueJobParams{
		Type:    jobType,
		Payload: rawPayload,
		AvailableAt: pgtype.Timestamptz{
			Time:  availableAt,
			Valid: true,
		},
	})
	if err != nil {
		return Job{}, err
	}
	return fromDBJob(job), nil
}

func (q *Queue) Claim(ctx context.Context, workerID string) (Job, error) {
	job, err := q.queries.ClaimJob(ctx, pgtype.Text{
		String: workerID,
		Valid:  true,
	})
	if err != nil {
		return Job{}, err
	}
	return fromDBJob(job), nil
}

func (q *Queue) Complete(ctx context.Context, jobID int64) error {
	return q.queries.CompleteJob(ctx, jobID)
}

func (q *Queue) Fail(ctx context.Context, jobID int64, message string, availableAt time.Time) error {
	return q.queries.FailJob(ctx, dbgen.FailJobParams{
		ID: jobID,
		LastError: pgtype.Text{
			String: message,
			Valid:  true,
		},
		AvailableAt: pgtype.Timestamptz{
			Time:  availableAt,
			Valid: true,
		},
	})
}

func fromDBJob(job dbgen.Job) Job {
	return Job{
		ID:          job.ID,
		Type:        job.Type,
		Payload:     job.Payload,
		Attempts:    job.Attempts,
		AvailableAt: job.AvailableAt.Time,
	}
}
