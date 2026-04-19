package storage

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deependra191/algoedgefno-backend/internal/entities"
	"github.com/deependra191/algoedgefno-backend/internal/models"
)

var _ models.UserRepository = (*UserStore)(nil)

type UserStore struct {
	pool *pgxpool.Pool
}

func NewUserStore(pool *pgxpool.Pool) *UserStore {
	return &UserStore{pool: pool}
}

func (r *UserStore) Create(ctx context.Context, user *models.User, passwordHash string) error {
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO users (id, email, name, password_hash, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, NOW(), NOW())`,
		user.ID, user.Email, user.Name, passwordHash,
	)
	return err
}

// FindByEmail returns the domain user and their password hash.
// Returns models.ErrNotFound if no user exists with that email.
func (r *UserStore) FindByEmail(ctx context.Context, email string) (*models.User, string, error) {
	var u entities.User
	err := r.pool.QueryRow(ctx,
		`SELECT id, email, name, password_hash, created_at, updated_at
		 FROM users WHERE email = $1`, email,
	).Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", models.ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	return fromUserEntity(&u), u.PasswordHash, nil
}

func (r *UserStore) FindByID(ctx context.Context, id string) (*entities.User, error) {
	var u entities.User
	err := r.pool.QueryRow(ctx,
		`SELECT id, email, name, password_hash, created_at, updated_at
		 FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func fromUserEntity(e *entities.User) *models.User {
	return &models.User{
		ID:        e.ID,
		Email:     e.Email,
		Name:      e.Name,
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
	}
}
