package services

import (
	"context"
	"errors"
	"time"

	"github.com/deependra191/algoedgefno-backend/internal/entities"
	"github.com/deependra191/algoedgefno-backend/internal/models"
	"github.com/deependra191/algoedgefno-backend/internal/storage"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

type AuthService struct {
	userRepo  *storage.UserStore
	jwtSecret []byte
}

func NewAuthService(userRepo *storage.UserStore, jwtSecret string) *AuthService {
	return &AuthService{
		userRepo:  userRepo,
		jwtSecret: []byte(jwtSecret),
	}
}

type RegisterInput struct {
	Email    string
	Password string
	Name     string
}

type LoginInput struct {
	Email    string
	Password string
}

type AuthResult struct {
	Token string
	User  *models.User
}

func (s *AuthService) Register(ctx context.Context, input RegisterInput) (*AuthResult, error) {
	_, err := s.userRepo.FindByEmail(ctx, input.Email)
	if err == nil {
		return nil, errors.New("email already registered")
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, errors.New("failed to check existing user")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcryptCost)
	if err != nil {
		return nil, errors.New("failed to process password")
	}

	ent := &entities.User{
		ID:           uuid.New(),
		Email:        input.Email,
		Name:         input.Name,
		PasswordHash: string(hash),
	}

	if err := s.userRepo.Create(ctx, ent); err != nil {
		return nil, errors.New("failed to create user")
	}

	token, err := s.generateToken(ent.ID.String())
	if err != nil {
		return nil, errors.New("failed to generate token")
	}

	return &AuthResult{Token: token, User: models.FromUserEntity(ent)}, nil
}

const (
	// bcryptCost is the work factor for password hashing.
	// Higher = slower hashing = more expensive brute force. 12 ≈ 250ms on modern hardware.
	bcryptCost = 12

	// tokenExpiry is how long a JWT remains valid after issue.
	tokenExpiry = 24 * time.Hour

	// jwtSubClaim is the JWT claim key used to store and retrieve the user ID.
	// Must match in both generateToken and ValidateToken.
	jwtSubClaim = "sub"

	// dummyHash is used in Login to ensure bcrypt runs even when a user is not found,
	// preventing user enumeration via response timing differences.
	dummyHash = "$2a$12$dummy.hash.for.timing.mitigation.only.not.a.real.hash.."
)

func (s *AuthService) Login(ctx context.Context, input LoginInput) (*AuthResult, error) {
	user, lookupErr := s.userRepo.FindByEmail(ctx, input.Email)

	// Always run bcrypt regardless of whether the user exists.
	// This prevents an attacker from enumerating valid emails by measuring
	// response time — bcrypt without this check returns instantly for unknown emails.
	hashToCheck := dummyHash
	if lookupErr == nil {
		hashToCheck = user.PasswordHash
	}
	bcryptErr := bcrypt.CompareHashAndPassword([]byte(hashToCheck), []byte(input.Password))

	if lookupErr != nil {
		if errors.Is(lookupErr, pgx.ErrNoRows) {
			return nil, errors.New("invalid credentials")
		}
		return nil, errors.New("failed to find user")
	}
	if bcryptErr != nil {
		return nil, errors.New("invalid credentials")
	}

	token, err := s.generateToken(user.ID.String())
	if err != nil {
		return nil, errors.New("failed to generate token")
	}

	return &AuthResult{Token: token, User: models.FromUserEntity(user)}, nil
}

func (s *AuthService) ValidateToken(tokenStr string) (string, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return s.jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return "", errors.New("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("invalid token claims")
	}

	userID, ok := claims[jwtSubClaim].(string)
	if !ok {
		return "", errors.New("invalid token subject")
	}

	return userID, nil
}

func (s *AuthService) generateToken(userID string) (string, error) {
	claims := jwt.MapClaims{
		jwtSubClaim: userID,
		"exp":       time.Now().Add(tokenExpiry).Unix(),
		"iat":       time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}
