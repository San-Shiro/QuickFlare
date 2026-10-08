package supervisor

import (
	"strings"
	"testing"
)

func TestParsePrometheusMetrics(t *testing.T) {
	raw := `
# HELP cloudflared_tunnel_total_requests Total number of requests processed
# TYPE cloudflared_tunnel_total_requests counter
cloudflared_tunnel_total_requests 142
# HELP cloudflared_tunnel_ha_connections Current number of HA connections
# TYPE cloudflared_tunnel_ha_connections gauge
cloudflared_tunnel_ha_connections 4
# HELP cloudflared_tunnel_active_streams Active stream count
# TYPE cloudflared_tunnel_active_streams gauge
cloudflared_tunnel_active_streams 3
# HELP cloudflared_tunnel_response_by_code Breakdown of responses
cloudflared_tunnel_response_by_code{status_code="200"} 120
cloudflared_tunnel_response_by_code{status_code="204"} 5
cloudflared_tunnel_response_by_code{status_code="404"} 12
cloudflared_tunnel_response_by_code{status_code="502"} 5
cloudflared_tunnel_response_by_code{status_code="301"} 10
invalid line
# comment
`
	tel := ParsePrometheusMetrics(strings.NewReader(raw))

	if tel.TotalRequests != 142 {
		t.Errorf("TotalRequests = %d, want 142", tel.TotalRequests)
	}
	if tel.HAConnections != 4 {
		t.Errorf("HAConnections = %d, want 4", tel.HAConnections)
	}
	if tel.ActiveStreams != 3 {
		t.Errorf("ActiveStreams = %d, want 3", tel.ActiveStreams)
	}
	if tel.Status2xx != 125 {
		t.Errorf("Status2xx = %d, want 125", tel.Status2xx)
	}
	if tel.Status4xx != 12 {
		t.Errorf("Status4xx = %d, want 12", tel.Status4xx)
	}
	if tel.Status5xx != 5 {
		t.Errorf("Status5xx = %d, want 5", tel.Status5xx)
	}
}
