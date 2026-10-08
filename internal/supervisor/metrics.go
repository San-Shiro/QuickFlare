package supervisor

import (
	"bufio"
	"io"
	"strconv"
	"strings"
)

// Telemetry holds aggregated tunnel metrics scraped from cloudflared.
type Telemetry struct {
	TotalRequests int64 `json:"total_requests"`
	ActiveStreams int64 `json:"active_streams"`
	HAConnections int   `json:"ha_connections"`
	Status2xx     int64 `json:"status_2xx"`
	Status4xx     int64 `json:"status_4xx"`
	Status5xx     int64 `json:"status_5xx"`
}

// ParsePrometheusMetrics parses cloudflared metrics lines into a Telemetry snapshot.
func ParsePrometheusMetrics(r io.Reader) Telemetry {
	var tel Telemetry
	sc := bufio.NewScanner(r)

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		key := parts[0]
		valStr := parts[len(parts)-1]

		switch {
		case strings.HasPrefix(key, "cloudflared_tunnel_total_requests"):
			if v, err := strconv.ParseInt(valStr, 10, 64); err == nil {
				tel.TotalRequests = v
			}
		case strings.HasPrefix(key, "cloudflared_tunnel_ha_connections"):
			if v, err := strconv.Atoi(valStr); err == nil {
				tel.HAConnections = v
			}
		case strings.HasPrefix(key, "cloudflared_tunnel_active_streams"):
			if v, err := strconv.ParseInt(valStr, 10, 64); err == nil {
				tel.ActiveStreams = v
			}
		case strings.HasPrefix(key, "cloudflared_tunnel_response_by_code"):
			if v, err := strconv.ParseInt(valStr, 10, 64); err == nil {
				if strings.Contains(key, `status_code="2`) {
					tel.Status2xx += v
				} else if strings.Contains(key, `status_code="4`) {
					tel.Status4xx += v
				} else if strings.Contains(key, `status_code="5`) {
					tel.Status5xx += v
				}
			}
		}
	}
	return tel
}
