// Command climatesnap downloads a daily ERA5 series from the Open-Meteo Historical Weather API once
// and writes it as an offline snapshot (datasets/environmental/*.json) with its provenance.
//
//	cd backend && go run -tags "sqlite_fts5 sqlite_rtree" ./internal/simulation/cmd/climatesnap \
//	    -lat 41.38 -lon 33.78 -place "Kastamonu (city centre)" -from 1991-01-01 -to 2025-12-31 \
//	    -out ../datasets/environmental/kastamonu_era5_daily_1991_2025.json
//
// Free-API terms (non-commercial, rate limits) apply: see docs/data_sources/climate.md.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/config"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/simulation/climate"
)

func main() {
	lat := flag.Float64("lat", 41.38, "latitude")
	lon := flag.Float64("lon", 33.78, "longitude")
	place := flag.String("place", "Kastamonu (city centre)", "free-text place label stored in the file")
	from := flag.String("from", "1991-01-01", "first day (YYYY-MM-DD)")
	to := flag.String("to", "2025-12-31", "last day (YYYY-MM-DD)")
	out := flag.String("out", "", "output file (required)")
	flag.Parse()
	if *out == "" {
		fmt.Fprintln(os.Stderr, "-out is required")
		os.Exit(2)
	}
	f, err := time.Parse("2006-01-02", *from)
	if err != nil {
		fatal(err)
	}
	t, err := time.Parse("2006-01-02", *to)
	if err != nil {
		fatal(err)
	}
	fetcher := climate.NewFetcher(config.Load().Climate)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	ser, err := fetcher.FetchDaily(ctx, *lat, *lon, f, t)
	if err != nil {
		fatal(err)
	}
	sn, err := climate.SnapshotFromSeries(ser, *place)
	if err != nil {
		fatal(err)
	}
	if err := climate.WriteSnapshot(*out, sn); err != nil {
		fatal(err)
	}
	var np, nt int
	for i := range ser.Dates {
		if ser.Precip[i] == ser.Precip[i] {
			np++
		}
		if ser.Temp[i] == ser.Temp[i] {
			nt++
		}
	}
	fmt.Printf("wrote %s: %d days %s..%s, grid %.4fN %.4fE, elevation %.0f m, precip values %d, temp values %d, retrieved %s\n",
		*out, len(ser.Dates), sn.StartDate, sn.EndDate, ser.GridLat, ser.GridLon, ser.ElevationM, np, nt, sn.RetrievedAt)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
