# meshcore-bots

A Go binary that sends scheduled weather, tide, sun, frost, and traffic updates to a [RemoteTerm](https://github.com/fdlamotte/Remote-Terminal-for-MeshCore) instance over your MeshCore mesh radio network.

## Bots

| Bot | Channel | Scope | Schedule |
|---|---|---|---|
| WeatherBot | `#weather` | fif, edi, tay | 07:00 daily |
| SunBot | `#weather` | fif, edi, tay | 07:00 daily |
| TideBot | `#weather` | fif, edi, tay | 07:00 daily |
| FrostBot | `#traffic` | frost regions only | 07:00 + 17:00 daily |
| TrafficBot | `#traffic` | fif, edi, tay | 07:00 daily + 15-min poll |

Each message is scoped to only flood the relevant region (fif / edi / tay) — nodes in other regions do not receive it.

## Prerequisites

- Go 1.22+
- A running RemoteTerm instance (self-signed TLS is fine — TLS verification is skipped)
- A free [Stormglass](https://stormglass.io) API key for WeatherBot, TideBot, and FrostBot (10 requests/day free tier — optional)
- Your RemoteTerm radio client renamed to **ScotBot** (or similar) so mesh recipients see a sensible sender name

## Setup

### 1. Clone and configure

```bash
git clone https://github.com/your-user/meshcore-bots
cd meshcore-bots
cp config.example.yaml config.yaml
```

Edit `config.yaml`:

- Set `remoteterm_url` to your RemoteTerm instance (e.g. `https://10.7.44.19:8000`)
- Set `tide_api_key` to your WorldTides API key, or leave as-is to skip TideBot
- Adjust `regions`, `frost_threshold_c`, and `max_messages_per_minute` as needed

### 2. Get a Stormglass API key (optional — for WeatherBot, TideBot, FrostBot)

1. Register free at [https://stormglass.io](https://stormglass.io)
2. Confirm your email and copy your API key from the dashboard
3. Paste it into `config.yaml` under `stormglass_api_key`

The free tier provides **10 requests/day**. With 3 regions: 3 weather + 3 tide = **6 requests/day** — comfortably within the limit, with 4 spare for restarts covered by the daily file cache. SunBot and TrafficBot always run regardless of whether a key is configured.

### 3. Build

```bash
go mod tidy
go build -o meshcore-bots ./cmd/bots
```

### 4. Test with dry-run

```bash
./meshcore-bots --dry-run
```

This prints every message that _would_ be sent, with its scope and channel, without touching RemoteTerm. No config or network connection required.

## CLI flags

| Flag | Behaviour |
|---|---|
| *(none)* | Normal mode — bootstrap channels, send first-boot messages, then run scheduler |
| `--dry-run` | Fetch all data, print every message to stdout, **do not send** |
| `--run-now` | Bootstrap, send all morning messages once immediately, then exit |
| `--dry-run --run-now` | Fetch data and print what `--run-now` would send, then exit |
| `--config path` | Path to config file (default: `config.yaml`) |

`--run-now` is useful when the service starts after 07:00 and you want to re-trigger the morning set without waiting.

## Deploy to server

Cross-compile on your Mac and copy to the server:

```bash
# Build for Linux
GOOS=linux GOARCH=amd64 go build -o meshcore-bots ./cmd/bots

# Copy files
scp meshcore-bots config.yaml scripts/install-service.sh alex@10.7.44.19:~/meshcore-bots/

# On the server — install and start as a systemd service
bash ~/meshcore-bots/install-service.sh
```

### Following logs

```bash
sudo journalctl -u meshcore-bots -f
```

### Managing the service

```bash
sudo systemctl status meshcore-bots
sudo systemctl restart meshcore-bots
sudo systemctl stop meshcore-bots
```

## Configuration reference

```yaml
remoteterm_url: https://10.7.44.19:8000   # RemoteTerm base URL (TLS skip-verify)

weather_channel: "#weather"
traffic_channel: "#traffic"

daily_time: "07:00"                        # Morning run (cron: 0 7 * * *)
frost_evening_time: "17:00"               # Evening frost check
poll_interval_minutes: 15                  # Traffic poll interval

max_messages_per_minute: 4                 # Global send rate (max 10)
frost_threshold_c: 2                       # Frost warning below this °C

stormglass_api_key: "YOUR_STORMGLASS_API_KEY"  # Drives WeatherBot, TideBot, FrostBot

regions:
  - scope: fif                             # MeshCore flood scope label
    label: Fife                            # Human-readable region name
    lat: 56.198                            # Latitude (for sun + tide calculations)
    lon: -3.176                            # Longitude
    weather_location: Glenrothes          # Location string for wttr.in
    tide_station: Rosyth                   # Display name on tide messages
    traffic_roads:                         # Road names to monitor
      - M90
      - A92
      - Forth Road Bridge
```

## Data sources

| Bot | Source | API key? | Rate limit |
|---|---|---|---|
| WeatherBot, TideBot, FrostBot | [stormglass.io](https://stormglass.io) | Yes (free) | 10 req/day (6 used) |
| SunBot | Built-in maths (NOAA algorithm) | None | N/A |
| TrafficBot | [traffic.gov.scot](https://www.traffic.gov.scot/traffic-information/incidents) | None | Unlimited |

## Message formats

```
☁️ Fife: Partly cloudy, Hi 14°C Lo 7°C, Rain 25%, SW 18km/h
🌅 Fife: Rise 05:12, Set 21:48 · 16h 36m daylight
🌊 Rosyth: HW 08:23 5.1m · LW 14:47 0.8m · HW 21:02 5.0m
🧊 Fife: Frost risk this morning (-1°C overnight). Check road conditions.
🧊 Fife: Frost forecast tonight (low -2°C). Expect icy roads tomorrow.
🚦 Fife: M90 J6 lane closure. A92 Tay Bridge no dbl-deck buses. +1 more
🟢 Edinburgh: No incidents on monitored roads
⚠️ Fife: New — M90 J3 southbound lane closed
```

## Rate limiting

All messages pass through a single `ScopedSendQueue`. The drain goroutine enforces a minimum interval of `60s / max_messages_per_minute` between sends. A typical 15-message morning run (3 weather + 3 sun + 3 tide + 3 traffic + 0–3 frost) at 4 msg/min completes in under 4 minutes.

The scope override (regional targeting) is set and cleared atomically around each send — scope is applied immediately before sending, not at enqueue time.
