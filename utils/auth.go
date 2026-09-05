package utils

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

const (
	AuthCookieName           = "codekanban_auth"
	FrontendPBKDF2Iterations = 20000

	argon2Version   = 19
	argon2MemoryKiB = 32 * 1024
	argon2TimeCost  = 2
	argon2Threads   = 1
	argon2SaltBytes = 16
	argon2KeyBytes  = 32
)

var (
	ErrInvalidSessionToken  = errors.New("invalid session token")
	ErrExpiredSessionToken  = errors.New("expired session token")
	ErrInvalidPasswordHash  = errors.New("invalid password hash")
	ErrUnsupportedHashAlgo  = errors.New("unsupported password hash algorithm")
	ErrUnsupportedHashValue = errors.New("unsupported password hash version")
)

type AuthSessionClaims struct {
	IssuedAt  int64 `json:"iat"`
	ExpiresAt int64 `json:"exp"`
}

func AuthEnabled(cfg *AppConfig) bool {
	if cfg == nil {
		return false
	}
	return strings.TrimSpace(cfg.Auth.PasswordHash) != ""
}

func EnsureAuthConfig(cfg *AppConfig) error {
	if cfg == nil {
		return nil
	}

	var frontendSalt string
	var tokenSecret string
	if strings.TrimSpace(cfg.Auth.FrontendSalt) == "" {
		value, err := NewAuthFrontendSalt()
		if err != nil {
			return err
		}
		frontendSalt = value
	}
	if strings.TrimSpace(cfg.Auth.TokenSecret) == "" {
		value, err := NewAuthTokenSecret()
		if err != nil {
			return err
		}
		tokenSecret = value
	}

	if frontendSalt == "" && tokenSecret == "" {
		cfg.Auth = SanitizeAuthConfig(cfg.Auth)
		cfg.Auth.sessionDuration = 0
		_ = cfg.Auth.SessionDuration()
		return nil
	}

	return UpdateConfig(cfg, func(current *AppConfig) {
		current.Auth = SanitizeAuthConfig(current.Auth)
		if strings.TrimSpace(current.Auth.FrontendSalt) == "" {
			current.Auth.FrontendSalt = frontendSalt
		}
		if strings.TrimSpace(current.Auth.TokenSecret) == "" {
			current.Auth.TokenSecret = tokenSecret
		}
		current.Auth.sessionDuration = 0
		_ = current.Auth.SessionDuration()
	})
}

func NewAuthFrontendSalt() (string, error) {
	return randomURLToken(24)
}

func NewAuthTokenSecret() (string, error) {
	return randomURLToken(32)
}

func IssueAuthSessionToken(secret string, ttl time.Duration, now time.Time) (string, error) {
	if strings.TrimSpace(secret) == "" {
		return "", ErrInvalidSessionToken
	}

	claims := AuthSessionClaims{
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(ttl).Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	payloadPart := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payloadPart))
	signaturePart := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return payloadPart + "." + signaturePart, nil
}

func VerifyAuthSessionToken(token string, secret string, now time.Time) (*AuthSessionClaims, error) {
	token = strings.TrimSpace(token)
	if token == "" || strings.TrimSpace(secret) == "" {
		return nil, ErrInvalidSessionToken
	}

	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return nil, ErrInvalidSessionToken
	}

	payloadPart := parts[0]
	signaturePart := parts[1]
	if payloadPart == "" || signaturePart == "" {
		return nil, ErrInvalidSessionToken
	}

	signature, err := base64.RawURLEncoding.DecodeString(signaturePart)
	if err != nil {
		return nil, ErrInvalidSessionToken
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payloadPart))
	expected := mac.Sum(nil)
	if subtle.ConstantTimeCompare(signature, expected) != 1 {
		return nil, ErrInvalidSessionToken
	}

	payload, err := base64.RawURLEncoding.DecodeString(payloadPart)
	if err != nil {
		return nil, ErrInvalidSessionToken
	}

	var claims AuthSessionClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, ErrInvalidSessionToken
	}
	if claims.ExpiresAt <= now.Unix() {
		return nil, ErrExpiredSessionToken
	}
	return &claims, nil
}

func HashClientSecret(clientHash string) (string, error) {
	if strings.TrimSpace(clientHash) == "" {
		return "", ErrInvalidPasswordHash
	}

	salt := make([]byte, argon2SaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	hash := argon2.IDKey(
		[]byte(clientHash),
		salt,
		argon2TimeCost,
		argon2MemoryKiB,
		argon2Threads,
		argon2KeyBytes,
	)

	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2Version,
		argon2MemoryKiB,
		argon2TimeCost,
		argon2Threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func VerifyClientSecret(clientHash string, encoded string) (bool, error) {
	if strings.TrimSpace(clientHash) == "" || strings.TrimSpace(encoded) == "" {
		return false, ErrInvalidPasswordHash
	}

	params, salt, expectedHash, err := parseArgon2Hash(encoded)
	if err != nil {
		return false, err
	}

	hash := argon2.IDKey(
		[]byte(clientHash),
		salt,
		params.timeCost,
		params.memoryKiB,
		params.threads,
		uint32(len(expectedHash)),
	)

	return subtle.ConstantTimeCompare(hash, expectedHash) == 1, nil
}

type argon2Params struct {
	memoryKiB uint32
	timeCost  uint32
	threads   uint8
}

func parseArgon2Hash(encoded string) (*argon2Params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		return nil, nil, nil, ErrInvalidPasswordHash
	}
	if parts[1] != "argon2id" {
		return nil, nil, nil, ErrUnsupportedHashAlgo
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return nil, nil, nil, ErrInvalidPasswordHash
	}
	if version != argon2Version {
		return nil, nil, nil, ErrUnsupportedHashValue
	}

	params := &argon2Params{}
	var threads int
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &params.memoryKiB, &params.timeCost, &threads); err != nil {
		return nil, nil, nil, ErrInvalidPasswordHash
	}
	if threads <= 0 || threads > 255 {
		return nil, nil, nil, ErrInvalidPasswordHash
	}
	params.threads = uint8(threads)

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return nil, nil, nil, ErrInvalidPasswordHash
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return nil, nil, nil, ErrInvalidPasswordHash
	}
	if len(salt) == 0 || len(hash) == 0 {
		return nil, nil, nil, ErrInvalidPasswordHash
	}
	return params, salt, hash, nil
}

func randomURLToken(size int) (string, error) {
	if size <= 0 {
		size = 16
	}
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
