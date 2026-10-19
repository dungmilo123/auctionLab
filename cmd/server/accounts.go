package main

import (
	"auctionlab/internal/account"
	"auctionlab/internal/store"
	"context"
	"encoding/json"
	"errors"
	"io"
)

func validateAccountSeedMode(accounts, catalog bool, importPath string, verify bool, seedAt string, tunnel bool) error {
	if accounts && (catalog || importPath != "" || verify || seedAt != "") {
		return errors.New("--seed-accounts is mutually exclusive with catalog seed and import flags")
	}
	if accounts && tunnel {
		return errors.New("maintenance mode cannot start a tunnel")
	}
	return nil
}
func seedAccountMaintenance(st *store.Store, getenv func(string) string, out io.Writer) error {
	s := account.New(store.AccountRepository{Store: st})
	done, e := s.Repo.Seeded(context.Background())
	if e != nil {
		return e
	}
	if !done {
		done, e = s.Provision(context.Background(), [3]string{getenv("DEMO_BIDDER_ONE_PASSWORD"), getenv("DEMO_BIDDER_TWO_PASSWORD"), getenv("DEMO_ADMIN_PASSWORD")})
		if e != nil {
			return e
		}
	}
	status := "seeded"
	if done {
		status = "already_applied"
	}
	return json.NewEncoder(out).Encode(map[string]string{"status": status})
}
