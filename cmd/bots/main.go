package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/robfig/cron/v3"

	"meshcore-bots/internal/bootstrap"
	"meshcore-bots/internal/config"
	"meshcore-bots/internal/frost"
	"meshcore-bots/internal/queue"
	"meshcore-bots/internal/remoteterm"
	"meshcore-bots/internal/state"
	"meshcore-bots/internal/sun"
	"meshcore-bots/internal/tide"
	"meshcore-bots/internal/traffic"
	"meshcore-bots/internal/weather"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "Fetch all data and print messages without sending to RemoteTerm")
	runNow := flag.Bool("run-now", false, "Send the full morning set once immediately then exit (no scheduler)")
	configPath := flag.String("config", "config.yaml", "Path to config file")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[meshcore-bots] ")

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	client := remoteterm.NewClient(cfg.RemotetermURL)

	weatherKey := remoteterm.HashtagChannelKey(cfg.WeatherChannel)
	trafficKey := remoteterm.HashtagChannelKey(cfg.TrafficChannel)

	channelNames := map[string]string{
		weatherKey: cfg.WeatherChannel,
		trafficKey: cfg.TrafficChannel,
	}

	q := queue.New(client, cfg.MaxMessagesPerMinute, *dryRun, channelNames)

	trafficState := state.New()
	wxCache := weather.NewCache("weather-cache.json")
	tideCache := tide.NewCache("tide-cache.json")

	// Bootstrap: wait for RemoteTerm, ensure channels exist.
	// Skipped in dry-run mode (no RemoteTerm connection required).
	if !*dryRun {
		if err := bootstrap.Run(client, []string{cfg.WeatherChannel, cfg.TrafficChannel}); err != nil {
			log.Fatalf("Bootstrap failed: %v", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go q.Start(ctx)

	// First-boot / --run-now: send all morning messages immediately.
	log.Println("Running morning bots (first boot / on-demand)...")
	runMorning(q, cfg, weatherKey, trafficKey, trafficState, wxCache, tideCache)

	if *runNow || *dryRun {
		// Wait for the queue to drain, then exit.
		if !*dryRun {
			log.Println("Waiting for queue to drain...")
			q.Drain()
		}
		log.Println("Done.")
		return
	}

	// Build and start the cron scheduler.
	c := cron.New()

	dailyCron, err := timeToCron(cfg.DailyTime)
	if err != nil {
		log.Fatalf("Invalid daily_time %q: %v", cfg.DailyTime, err)
	}
	if _, err := c.AddFunc(dailyCron, func() {
		log.Println("Cron: daily morning run")
		trafficState.Reset()
		runMorning(q, cfg, weatherKey, trafficKey, trafficState, wxCache, tideCache)
	}); err != nil {
		log.Fatalf("Adding daily cron: %v", err)
	}

	eveningCron, err := timeToCron(cfg.FrostEveningTime)
	if err != nil {
		log.Fatalf("Invalid frost_evening_time %q: %v", cfg.FrostEveningTime, err)
	}
	if _, err := c.AddFunc(eveningCron, func() {
		log.Println("Cron: frost evening check")
		runFrostEvening(q, cfg, trafficKey, wxCache)
	}); err != nil {
		log.Fatalf("Adding frost evening cron: %v", err)
	}

	pollCron := fmt.Sprintf("*/%d * * * *", cfg.PollIntervalMinutes)
	if _, err := c.AddFunc(pollCron, func() {
		runTrafficPoll(q, cfg, trafficKey, trafficState)
	}); err != nil {
		log.Fatalf("Adding traffic poll cron: %v", err)
	}

	c.Start()
	log.Printf("Scheduler started. Daily at %s, frost check at %s, traffic poll every %d min.",
		cfg.DailyTime, cfg.FrostEveningTime, cfg.PollIntervalMinutes)
	log.Printf("Rate limit: %d message(s)/min (one every %ds).",
		cfg.MaxMessagesPerMinute, 60/cfg.MaxMessagesPerMinute)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	<-sig

	log.Println("Shutting down — waiting for in-flight send to complete...")
	c.Stop()
	cancel()
	q.Drain()
	log.Println("Goodbye.")
}

// runMorning sends the full set of morning messages: weather, sun, tide, frost, traffic.
func runMorning(q *queue.Queue, cfg *config.Config, weatherKey, trafficKey string, st *state.IncidentState, wxCache *weather.Cache, tideCache *tide.Cache) {
	// Fetch traffic once for all regions.
	allIncidents, err := traffic.FetchAll()
	if err != nil {
		log.Printf("WARN traffic fetch failed: %v", err)
	}

	sgKey := cfg.StormglassAPIKey
	hasSG := sgKey != "" && sgKey != "YOUR_STORMGLASS_API_KEY"

	for _, region := range cfg.Regions {
		// --- Weather via wttr.in (no API key needed, always attempted) ---
		var wd *weather.Data
		{
			var err error
			wd, err = weather.FetchCached(region.WeatherQuery(), wxCache)
			if err != nil {
				log.Printf("WARN weather fetch failed for %s: %v", region.Label, err)
			}
		}
		if wd != nil {
			q.Enqueue(region.Scope, weatherKey, weather.Format(region.Label, wd))

			// --- Frost morning check (reuses weather data) ---
			if frost.CheckMorning(wd, cfg.FrostThresholdC) {
				q.Enqueue(region.Scope, trafficKey, frost.FormatMorning(region.Label, wd))
			}
		}

		// --- Sun (pure maths, always runs) ---
		sd, err := sun.Compute(region.Lat, region.Lon, time.Now())
		if err != nil {
			log.Printf("WARN sun compute failed for %s: %v", region.Label, err)
		} else {
			q.Enqueue(region.Scope, weatherKey, sun.Format(region.Label, sd))
		}

		// --- Tide via Stormglass (cached; requires stormglass_api_key) ---
		if hasSG {
			extremes, err := tide.FetchCached(region.Label, region.Lat, region.Lon, sgKey, tideCache)
			if err != nil {
				log.Printf("WARN tide fetch failed for %s: %v", region.Label, err)
			} else {
				q.Enqueue(region.Scope, weatherKey, tide.Format(region.TideStation, extremes))
			}
		}

		// --- Traffic daily summary (lane restrictions / diversions suppressed) ---
		regionIncidents := traffic.FilterHighPriority(traffic.FilterByRoads(allIncidents, region.TrafficRoads))
		msg := traffic.DailySummary(region.Label, regionIncidents, st)
		q.Enqueue(region.Scope, trafficKey, msg)
	}
}

// runFrostEvening sends frost warnings for regions where tonight's forecast is below threshold.
func runFrostEvening(q *queue.Queue, cfg *config.Config, trafficKey string, wxCache *weather.Cache) {
	for _, region := range cfg.Regions {
		wd, err := weather.FetchCached(region.WeatherQuery(), wxCache)
		if err != nil {
			log.Printf("WARN weather fetch for frost evening failed for %s: %v", region.Label, err)
			continue
		}
		if frost.CheckEvening(wd, cfg.FrostThresholdC) {
			q.Enqueue(region.Scope, trafficKey, frost.FormatEvening(region.Label, wd))
		}
	}
}

// runTrafficPoll checks for new incidents and sends alerts only for genuinely new ones.
func runTrafficPoll(q *queue.Queue, cfg *config.Config, trafficKey string, st *state.IncidentState) {
	allIncidents, err := traffic.FetchAll()
	if err != nil {
		log.Printf("WARN traffic poll fetch failed: %v", err)
		return
	}

	for _, region := range cfg.Regions {
		regionIncidents := traffic.FilterHighPriority(traffic.FilterByRoads(allIncidents, region.TrafficRoads))
		newOnes := traffic.NewIncidents(regionIncidents, st)
		for _, inc := range newOnes {
			log.Printf("New incident [%s]: %s", region.Label, inc.Title)
			q.Enqueue(region.Scope, trafficKey, traffic.FormatAlert(region.Label, inc))
		}
	}
}

// timeToCron converts "HH:MM" to a standard 5-field cron expression.
func timeToCron(t string) (string, error) {
	parts := strings.SplitN(t, ":", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("expected HH:MM format")
	}
	hour, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || hour < 0 || hour > 23 {
		return "", fmt.Errorf("invalid hour %q", parts[0])
	}
	minute, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || minute < 0 || minute > 59 {
		return "", fmt.Errorf("invalid minute %q", parts[1])
	}
	return fmt.Sprintf("%d %d * * *", minute, hour), nil
}
