// Package auction executes published commerce lots independently of the game.
package auction

import (
	"auctionlab/internal/catalog"
	"context"
	"regexp"
	"time"
)

const BatchSize = 100
const HistoryLimit = 50

type Bid struct {
	Sequence    int64  `json:"sequence"`
	AmountMinor int64  `json:"amount_minor"`
	AcceptedAt  int64  `json:"accepted_at"`
	Alias       string `json:"alias"`
}
type Result struct {
	Outcome    string `json:"outcome"`
	ClosesAt   int64  `json:"closes_at"`
	RecordedAt int64  `json:"recorded_at"`
	Winner     *Bid   `json:"winner"`
}
type Snapshot struct {
	LotID              string  `json:"lot_id"`
	Revision           int64   `json:"revision"`
	State              string  `json:"auction_state"`
	OpensAt            int64   `json:"opens_at"`
	ClosesAt           int64   `json:"closes_at"`
	Currency           string  `json:"currency"`
	StartingPriceMinor int64   `json:"starting_price_minor"`
	BidIncrementMinor  int64   `json:"bid_increment_minor"`
	CurrentPriceMinor  *int64  `json:"current_price_minor"`
	NextMinimumMinor   *int64  `json:"next_minimum_minor"`
	AmountLimitReached bool    `json:"amount_limit_reached"`
	BidCount           int64   `json:"bid_count"`
	Leader             *Bid    `json:"leader"`
	History            []Bid   `json:"history"`
	HistoryHasMore     bool    `json:"history_has_more"`
	Result             *Result `json:"result"`
	ServerTime         int64   `json:"-"`
}
type History struct {
	Bids           []Bid  `json:"bids"`
	HasMore        bool   `json:"has_more"`
	BeforeSequence *int64 `json:"before_sequence"`
}
type Intent struct {
	LotID       string `json:"lot_id"`
	AmountMinor *int64 `json:"amount_minor"`
	RequestID   string `json:"request_id"`
}
type OwnBid struct {
	Sequence    int64  `json:"sequence"`
	AmountMinor int64  `json:"amount_minor"`
	AcceptedAt  int64  `json:"accepted_at"`
	RequestID   string `json:"request_id"`
}
type Receipt struct {
	LotID  string `json:"lot_id"`
	Bid    OwnBid `json:"bid"`
	Replay bool   `json:"replay"`
}
type Participation struct {
	LotID             string   `json:"lot_id"`
	Slug              string   `json:"slug"`
	Title             string   `json:"title"`
	Status            string   `json:"status"`
	Latest            *OwnBid  `json:"latest"`
	Bids              []OwnBid `json:"bids"`
	CurrentPriceMinor *int64   `json:"current_price_minor"`
	Revision          int64    `json:"revision"`
	CheckoutAvailable bool     `json:"checkout_available"`
}
type Error struct{ Code string }

func (e *Error) Error() string { return "auction: " + e.Code }
func Fail(code string) error   { return &Error{code} }

var requestPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func Validate(i Intent) error {
	if i.LotID == "" || !requestPattern.MatchString(i.RequestID) {
		return Fail("invalid_request")
	}
	if i.AmountMinor == nil || *i.AmountMinor < 0 || *i.AmountMinor > catalog.MaxMoney {
		return Fail("invalid_amount")
	}
	return nil
}
func Minimum(opening, increment int64, current *int64) (*int64, bool) {
	if current == nil {
		return &opening, false
	}
	if *current > catalog.MaxMoney-increment {
		return nil, true
	}
	v := *current + increment
	return &v, false
}

type Repository interface {
	Reconcile(context.Context, func() time.Time) (int, error)
	Place(context.Context, string, Intent, func() time.Time) (Receipt, error)
	Snapshot(context.Context, string, func() time.Time) (Snapshot, error)
	History(context.Context, string, int64) (History, error)
	Participation(context.Context, string, string, func() time.Time) ([]Participation, error)
}
type Service struct {
	Repo Repository
	Now  func() time.Time
}

func New(r Repository, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{r, now}
}
func (s *Service) Place(ctx context.Context, digest string, i Intent) (Receipt, error) {
	if e := Validate(i); e != nil {
		return Receipt{}, e
	}
	return s.Repo.Place(ctx, digest, i, s.Now)
}
func (s *Service) Reconcile(ctx context.Context) error {
	for {
		n, e := s.Repo.Reconcile(ctx, s.Now)
		if e != nil {
			return e
		}
		if n == 0 {
			return nil
		}
		if e = ctx.Err(); e != nil {
			return e
		}
	}
}

// Run does bounded work per tick and always joins via its caller-owned context.
func (s *Service) Run(ctx context.Context, onError func(error)) {
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if _, e := s.Repo.Reconcile(ctx, s.Now); e != nil && ctx.Err() == nil && onError != nil {
				onError(e)
			}
		}
	}
}
