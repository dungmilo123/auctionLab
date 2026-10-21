// Package account owns commerce identity independently of the retained game.
package account

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"auctionlab/internal/catalog"
)

const SessionLifetime = 30 * 24 * time.Hour

type Profile struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	Demo        bool   `json:"demo"`
}

// Record is private repository input. Never serialize it as an API response.
type Record struct {
	Profile
	Verifier  string `json:"-"`
	CreatedAt int64  `json:"-"`
}
type Error struct {
	Code   string            `json:"code"`
	Fields map[string]string `json:"fields,omitempty"`
}

func (e *Error) Error() string { return "account: " + e.Code }
func Fail(code string) error   { return &Error{Code: code} }
func Invalid(field string) error {
	return &Error{Code: "validation", Fields: map[string]string{field: "invalid"}}
}
func Code(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return "unavailable"
}
func Email(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) == 0 || len(value) > 254 {
		return "", Invalid("email")
	}
	for _, r := range value {
		if r > 127 || unicode.IsSpace(r) || unicode.IsControl(r) {
			return "", Invalid("email")
		}
	}
	a, e := mail.ParseAddress(value)
	if e != nil || a.Address != value || a.Name != "" {
		return "", Invalid("email")
	}
	return value, nil
}
func Name(value string) (string, error) {
	if !utf8.ValidString(value) {
		return "", Invalid("display_name")
	}
	value = strings.TrimSpace(value)
	n := utf8.RuneCountInString(value)
	if n < 1 || n > 80 {
		return "", Invalid("display_name")
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return "", Invalid("display_name")
		}
	}
	return value, nil
}
func Password(value string) error {
	n := utf8.RuneCountInString(value)
	if !utf8.ValidString(value) || n < 12 || n > 128 || len(value) > 512 {
		return Invalid("password")
	}
	return nil
}

type Repository interface {
	Credentials(context.Context, string) (Record, error)
	Register(context.Context, Record, string, string, int64) (Profile, error)
	Login(context.Context, string, string, string, int64) (Profile, error)
	Profile(context.Context, string, int64) (Profile, error)
	Update(context.Context, string, string, int64) (Profile, error)
	Logout(context.Context, string, int64) error
	Membership(context.Context, string, string, *bool, int64) (bool, error)
	Watchlist(context.Context, string, int64) ([]catalog.PublicLot, error)
	Seeded(context.Context) (bool, error)
	Seed(context.Context, []Record, int64) (bool, error)
}

type Service struct {
	Repo   Repository
	Now    func() time.Time
	Hash   func(string) (string, error)
	Verify func(string, string) bool
}

func New(repo Repository) *Service {
	return &Service{Repo: repo, Now: time.Now, Hash: HashPassword, Verify: VerifyPassword}
}
func (s *Service) Register(ctx context.Context, email, password, name, old string) (Profile, string, error) {
	email, e := Email(email)
	if e != nil {
		return Profile{}, "", e
	}
	name, e = Name(name)
	if e != nil {
		return Profile{}, "", e
	}
	if e = Password(password); e != nil {
		return Profile{}, "", e
	}
	verifier, e := s.Hash(password)
	if e != nil {
		return Profile{}, "", Fail("unavailable")
	}
	id, e := Random(24)
	if e != nil {
		return Profile{}, "", Fail("unavailable")
	}
	token, e := Random(32)
	if e != nil {
		return Profile{}, "", Fail("unavailable")
	}
	now := s.Now().UnixMilli()
	p, e := s.Repo.Register(ctx, Record{Profile: Profile{ID: id, Email: email, DisplayName: name, Role: "bidder"}, Verifier: verifier, CreatedAt: now}, Digest(token), Digest(old), now)
	if e != nil {
		return Profile{}, "", e
	}
	return p, token, nil
}
func (s *Service) Login(ctx context.Context, email, password, old string) (Profile, string, error) {
	email, e := Email(email)
	if e != nil {
		email = ""
	}
	row, e := s.Repo.Credentials(ctx, email)
	if e != nil && Code(e) != "not_found" {
		return Profile{}, "", e
	}
	verifier := row.Verifier
	if verifier == "" {
		verifier = DummyVerifier
	}
	valid := s.Verify(password, verifier)
	if !valid || row.ID == "" {
		return Profile{}, "", Fail("invalid_credentials")
	}
	token, e := Random(32)
	if e != nil {
		return Profile{}, "", Fail("unavailable")
	}
	p, e := s.Repo.Login(ctx, row.ID, Digest(token), Digest(old), s.Now().UnixMilli())
	if e != nil {
		return Profile{}, "", e
	}
	return p, token, nil
}
func (s *Service) Provision(ctx context.Context, passwords [3]string) (bool, error) {
	done, e := s.Repo.Seeded(ctx)
	if e != nil || done {
		return done, e
	}
	rows, e := s.PrepareSeed(passwords)
	if e != nil {
		return false, e
	}
	return s.Repo.Seed(ctx, rows, s.Now().UnixMilli())
}

// PrepareSeed validates and hashes operator credentials without database writes.
func (s *Service) PrepareSeed(passwords [3]string) ([]Record, error) {
	emails := []string{"bidder-one@demo.auctionlab.test", "bidder-two@demo.auctionlab.test", "operator@demo.auctionlab.test"}
	names := []string{"Demo bidder one", "Demo bidder two", "Demo operator"}
	rows := make([]Record, 3)
	for i, p := range passwords {
		if Password(p) != nil {
			return nil, Fail("seed_credentials_required")
		}
		v, e := s.Hash(p)
		if e != nil {
			return nil, Fail("unavailable")
		}
		id, e := Random(24)
		if e != nil {
			return nil, Fail("unavailable")
		}
		role := "bidder"
		if i == 2 {
			role = "admin"
		}
		rows[i] = Record{Profile: Profile{ID: id, Email: emails[i], DisplayName: names[i], Role: role, Demo: true}, Verifier: v, CreatedAt: s.Now().UnixMilli()}
	}
	return rows, nil
}
