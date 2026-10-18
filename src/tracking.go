package main

import (
	"MeadowWorksTools/src/db"
	"context"
	"log"
	"time"
)

// How far back ESI's history went when the user first signed in, so it was
// saved too.
const (
	miningBackfillDays   = 30 // mining ledger
	industryBackfillDays = 90 // completed industry jobs
	walletBackfillDays   = 30 // wallet journal, behind My Worth's earned and spent
)

// trackingDays is how many days of the user's data a page covers: the days
// since they first signed in plus the days ESI let the first refresh fetch.
func trackingDays(ctx context.Context, userID int64, backfillDays int64) int64 {
	first, err := db.LoadFirstSignIn(ctx, userID)
	if err != nil {
		log.Printf("trackingDays: first sign-in of user %d: %v", userID, err)
		return backfillDays
	}
	return int64(time.Since(first)/(24*time.Hour)) + backfillDays
}
