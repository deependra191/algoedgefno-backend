package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deependra191/algoedgefno-backend/internal/entities"
	"github.com/deependra191/algoedgefno-backend/internal/models"
)

// constraintUsersEmailKey is the Postgres unique-constraint name on
// users.email. Named per rule 17 (opaque external identifier; the reader
// must inspect migrations/000001_users.up.sql to confirm the name).
const constraintUsersEmailKey = "users_email_key"

// pgCodeUniqueViolation is the PostgreSQL SQLSTATE for a unique-constraint
// violation (unique_violation), returned in pgconn.PgError.Code.
const pgCodeUniqueViolation = "23505"

var _ models.UserRepository = (*UserStore)(nil)

// UserStore implements models.UserRepository using a pgxpool.Pool.
type UserStore struct {
	pool *pgxpool.Pool
}

// NewUserStore constructs a UserStore backed by the given connection pool.
func NewUserStore(pool *pgxpool.Pool) *UserStore {
	return &UserStore{pool: pool}
}

// UpsertByFirebaseUID inserts a new user row or updates the existing row that
// matches firebase_uid. Email is canonicalized to lowercase before writing
// (see §11 of the plan). Returns ErrIdentityConflict when the canonicalized
// email is already owned by a different firebase_uid (23505 on
// users_email_key).
func (s *UserStore) UpsertByFirebaseUID(ctx context.Context, in *models.User) (*models.User, error) {
	email := canonicalizeEmail(in.Email)
	var ent entities.User
	err := s.pool.QueryRow(ctx, `
		INSERT INTO users (id, firebase_uid, email, display_name, photo_url,
		                   last_login_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW(), NOW())
		ON CONFLICT (firebase_uid) DO UPDATE
		SET email         = EXCLUDED.email,
		    display_name  = EXCLUDED.display_name,
		    photo_url     = EXCLUDED.photo_url,
		    last_login_at = NOW(),
		    updated_at    = NOW()
		RETURNING id, email, firebase_uid, display_name, photo_url,
		          last_login_at, created_at, updated_at`,
		uuid.New(), in.FirebaseUID, email, nilIfEmpty(in.DisplayName), nilIfEmpty(in.PhotoURL),
	).Scan(
		&ent.ID, &ent.Email, &ent.FirebaseUID, &ent.DisplayName,
		&ent.PhotoURL, &ent.LastLoginAt, &ent.CreatedAt, &ent.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err, constraintUsersEmailKey) {
			return nil, models.ErrIdentityConflict
		}
		return nil, fmt.Errorf("upsert by firebase_uid: %w", err)
	}
	return toUserModel(&ent), nil
}

// FindByID returns the domain user for the given UUID primary key.
// Returns models.ErrNotFound when no row exists.
func (s *UserStore) FindByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	var ent entities.User
	err := s.pool.QueryRow(ctx, `
		SELECT id, email, firebase_uid, display_name, photo_url,
		       last_login_at, created_at, updated_at
		FROM users WHERE id = $1`, id,
	).Scan(
		&ent.ID, &ent.Email, &ent.FirebaseUID, &ent.DisplayName,
		&ent.PhotoURL, &ent.LastLoginAt, &ent.CreatedAt, &ent.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, models.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find user by id: %w", err)
	}
	return toUserModel(&ent), nil
}

// toUserModel converts an entities.User scan target to a domain model.
// DisplayName and PhotoURL are stored as nullable strings; they become empty
// strings on the domain object when NULL in the database.
func toUserModel(e *entities.User) *models.User {
	m := &models.User{
		ID:          e.ID,
		FirebaseUID: e.FirebaseUID,
		Email:       e.Email,
		CreatedAt:   e.CreatedAt,
	}
	if e.DisplayName != nil {
		m.DisplayName = *e.DisplayName
	}
	if e.PhotoURL != nil {
		m.PhotoURL = *e.PhotoURL
	}
	return m
}

// canonicalizeEmail normalizes an email address to lowercase with surrounding
// whitespace removed. Must be applied before every write so the unique
// constraint on users.email behaves case-insensitively.
func canonicalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// nilIfEmpty converts an empty string to a nil pointer so pgx stores NULL
// rather than an empty string for optional text columns.
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// isUniqueViolation reports whether err is a 23505 PostgreSQL unique-violation
// on one of the given constraint names.
func isUniqueViolation(err error, constraints ...string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	if pgErr.Code != pgCodeUniqueViolation {
		return false
	}
	for _, c := range constraints {
		if pgErr.ConstraintName == c {
			return true
		}
	}
	return false
}
