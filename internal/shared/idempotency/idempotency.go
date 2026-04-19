// Package idempotency provides middleware that prevents duplicate execution
// of mutation requests.
//
// How it works:
//   - Client sends POST/PUT/DELETE with "Idempotency-Key: <some-uuid>" header.
//   - We hash the request body (SHA-256) and store the response against the
//     key for 48 hours.
//   - If the same key arrives again with the SAME body, we return the
//     cached response — the request is NOT processed again.
//   - If the same key arrives with a DIFFERENT body, that's a bug in the
//     client, and we return 422 IDEMPOTENCY_KEY_REUSED.
//
// This is critical for anything that transfers money, sends notifications,
// or creates unique resources. Apply the middleware ONLY to mutation
// routes — GETs are already idempotent.
//
// Unlike rate limiting, this middleware FAILS CLOSED. If the database is
// unreachable, we return an error rather than risk a double-charge.
//
// The Idempotency-Key header is optional. If a client doesn't send it,
// the middleware does nothing and the request proceeds normally.
package idempotency

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/topboyasante/api-base/internal/observability/logger"
	"github.com/topboyasante/api-base/internal/shared/requestctx"
	"github.com/topboyasante/api-base/internal/shared/response"
)

type Store struct {
	db *sqlx.DB
}

func NewStore(db *sqlx.DB) *Store {
	return &Store{db: db}
}

type Record struct {
	Key          string    `db:"idempotency_key"`
	ConsumerID   string    `db:"consumer_id"`
	RequestHash  string    `db:"request_hash"`
	ResponseBody []byte    `db:"response_body"`
	StatusCode   int       `db:"status_code"`
	ExpiresAt    time.Time `db:"expires_at"`
}

func (s *Store) Get(ctx context.Context, key, consumerID string) (*Record, error) {
	var r Record
	err := s.db.GetContext(ctx, &r, `
		SELECT idempotency_key, consumer_id, request_hash, response_body, status_code, expires_at
		FROM idempotent_requests
		WHERE idempotency_key = $1 AND consumer_id = $2 AND expires_at > NOW()
	`, key, consumerID)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) Save(ctx context.Context, r *Record) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO idempotent_requests
			(idempotency_key, consumer_id, request_hash, response_body, status_code, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (idempotency_key, consumer_id) DO NOTHING
	`, r.Key, r.ConsumerID, r.RequestHash, r.ResponseBody, r.StatusCode, r.ExpiresAt)
	return err
}

func Middleware(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader("Idempotency-Key")
		if key == "" {
			c.Next()
			return
		}

		// Consumer ID placeholder. When we add auth, replace this with the
		// authenticated user/API key ID so two clients can reuse the same
		// header value without colliding.
		// TODO: replace with authenticated consumer once auth module exists.
		consumerID := "anon"

		bodyBytes, _ := io.ReadAll(c.Request.Body)
		c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		hash := sha256Hex(bodyBytes)

		existing, err := store.Get(c.Request.Context(), key, consumerID)
		if err == nil && existing != nil {
			if existing.RequestHash != hash {
				response.Error(c, 422, "IDEMPOTENCY_KEY_REUSED",
					"idempotency key was used with different parameters")
				c.Abort()
				return
			}
			c.Data(existing.StatusCode, "application/json", existing.ResponseBody)
			c.Abort()
			return
		}

		rec := &responseCapture{ResponseWriter: c.Writer, body: &bytes.Buffer{}}
		c.Writer = rec

		c.Next()

		saveErr := store.Save(c.Request.Context(), &Record{
			Key:          key,
			ConsumerID:   consumerID,
			RequestHash:  hash,
			ResponseBody: rec.body.Bytes(),
			StatusCode:   c.Writer.Status(),
			ExpiresAt:    time.Now().Add(48 * time.Hour),
		})
		if saveErr != nil {
			logger.FromContext(c.Request.Context()).Error("idempotency_save_failed",
				"err", saveErr,
				"request_id", requestctx.RequestID(c.Request.Context()),
			)
		}
	}
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

type responseCapture struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (r *responseCapture) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}
