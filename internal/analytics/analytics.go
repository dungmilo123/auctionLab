// Package analytics describes lifetime aggregates from durable commerce facts.
package analytics

import "context"

type Filters struct {
	Event   string `json:"event"`
	Dataset string `json:"dataset"`
}
type Ratio struct {
	Sold     string `json:"sold"`
	Terminal string `json:"terminal"`
}
type Metrics struct {
	Lots            string `json:"lots"`
	Bids            string `json:"bids"`
	Bidders         string `json:"bidders"`
	DemoBids        string `json:"demo_bids"`
	OrdinaryBids    string `json:"non_demo_bids"`
	DemoBidders     string `json:"demo_bidders"`
	OrdinaryBidders string `json:"non_demo_bidders"`
	Sold            string `json:"sold"`
	Unsold          string `json:"unsold"`
	Terminal        string `json:"terminal"`
	Pending         string `json:"pending"`
	SellThrough     *Ratio `json:"sell_through"`
	Winning         string `json:"winning_minor"`
	PaidOrders      string `json:"paid_orders"`
	Unpaid          string `json:"unpaid_sold_orders"`
	PaidWinning     string `json:"paid_winning_minor"`
	PaidFees        string `json:"paid_fees_minor"`
	PaidTotal       string `json:"simulated_paid_minor"`
}
type Event struct {
	Slug    string  `json:"slug"`
	Title   string  `json:"title"`
	Demo    bool    `json:"demo"`
	Metrics Metrics `json:"metrics"`
}
type InteractionCount struct {
	Kind       string `json:"kind"`
	Provenance string `json:"provenance"`
	Count      string `json:"count"`
}
type Data struct {
	Summary      Metrics            `json:"summary"`
	Events       []Event            `json:"events"`
	Interactions []InteractionCount `json:"interactions"`
	Definitions  map[string]string  `json:"definitions"`
}
type Meta struct {
	Currency        string  `json:"currency"`
	AsOf            int64   `json:"as_of"`
	Filters         Filters `json:"filters"`
	CollectionStart *int64  `json:"collection_started_at"`
}
type Response struct {
	Data Data `json:"data"`
	Meta Meta `json:"meta"`
}
type Repository interface {
	Report(context.Context, Filters) (Response, error)
}

var Definitions = map[string]string{
	"cohort":       "Published lots in published events; lifetime facts attributed to their original event.",
	"demo":         "Lot demo OR event demo. Actor demo flags are a separate participation breakdown. All payments are simulated.",
	"bidders":      "Distinct accepted-bid accounts; per-event bidder counts are not additive.",
	"sell_through": "Sold / (sold + unsold); null for zero terminal results. Pending includes all lots without a committed result.",
	"winning":      "Sold-result winning amounts, including unpaid wins. Simulated paid = paid winning + delivery fees, once per receipt.",
	"coverage":     "Observed page views are received observations, not unique people or complete traffic. Backfilled actions and current watchlist snapshots are separate; no removed memberships or historical views are inferred.",
}
