package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/San-Shiro/QuickFlare/internal/cloudflare"
	"github.com/San-Shiro/QuickFlare/internal/config"
	"github.com/San-Shiro/QuickFlare/internal/core"
	"github.com/San-Shiro/QuickFlare/internal/ipc"
	"github.com/San-Shiro/QuickFlare/internal/supervisor"
)

// parseFlags parses flags that appear before OR after positional arguments.
//
// The stdlib flag package stops at the first non-flag word, so
// "route add app --port 3000" would leave --port unparsed and the port at
// zero - the exact form this tool documents. Parsing in a loop, peeling off
// one positional each time, accepts both orders.
func parseFlags(fs *flag.FlagSet, args []string) []string {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return positional
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return positional
		}
		positional = append(positional, rest[0])
		args = rest[1:]
	}
}

// out is a column-aligned writer for listings. Flush before returning.
func out() *tabwriter.Writer {
	return tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
}

func notifyTrayReload(ctx context.Context) {
	client, err := ipc.Discover()
	if err == nil && client != nil {
		_ = client.Reload(ctx)
	}
}

// ---------- start ----------

func cmdStart(ctx context.Context, args []string) error {
	client, err := ipc.Discover()
	if err == nil && client != nil {
		fmt.Printf("QuickFlare is already running (PID %d).\n", client.PID())
		_ = client.Open(ctx)
		return nil
	}

	fmt.Println("Starting QuickFlare...")
	if err := startTrayProcess(); err != nil {
		return err
	}

	// Poll until IPC responds or timeout
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(150 * time.Millisecond)
		if c, err := ipc.Discover(); err == nil && c != nil {
			fmt.Printf("QuickFlare started (PID %d).\n", c.PID())
			return nil
		}
	}

	fmt.Println("QuickFlare process launched; waiting for tray initialization.")
	return nil
}

// ---------- stop ----------

func cmdStop(ctx context.Context, args []string) error {
	client, err := ipc.Discover()
	if err != nil || client == nil {
		fmt.Println("QuickFlare is not running.")
		return nil
	}

	pid := client.PID()
	fmt.Printf("Stopping QuickFlare (PID %d)...\n", pid)
	if err := client.Quit(ctx); err != nil {
		return fmt.Errorf("send quit signal: %w", err)
	}

	// Wait for process to exit
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(150 * time.Millisecond)
		if _, err := ipc.Discover(); err != nil {
			fmt.Println("QuickFlare stopped.")
			return nil
		}
	}
	fmt.Println("QuickFlare stop signal sent.")
	return nil
}

// ---------- pause ----------

func cmdPause(ctx context.Context, args []string) error {
	client, err := ipc.Discover()
	if err == nil && client != nil {
		if err := client.Pause(ctx); err != nil {
			return fmt.Errorf("pause routes: %w", err)
		}
		fmt.Println("Routes paused.")
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.RoutesDisabled = true
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Println("Routes set to paused in settings (QuickFlare is currently not running).")
	return nil
}

// ---------- resume ----------

func cmdResume(ctx context.Context, args []string) error {
	client, err := ipc.Discover()
	if err == nil && client != nil {
		if err := client.Resume(ctx); err != nil {
			return fmt.Errorf("resume routes: %w", err)
		}
		fmt.Println("Routes resumed.")
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.RoutesDisabled = false
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Println("Routes set to active in settings (QuickFlare is currently not running).")
	return nil
}

// ---------- login ----------

func cmdLogin(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("login", flag.ExitOnError)
	token := fs.String("token", "", "API token (omit to be prompted; the prompt keeps it out of shell history)")
	domain := fs.String("domain", "", "default domain for new routes")
	fs.Parse(args)

	t := strings.TrimSpace(*token)
	if t == "" {
		// Read from stdin rather than requiring a flag: a token on the
		// command line ends up in shell history and in the process list,
		// where anyone on the machine can read it.
		fmt.Fprint(os.Stderr, "Cloudflare API token: ")
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && strings.TrimSpace(line) == "" {
			return fmt.Errorf("no token given")
		}
		t = strings.TrimSpace(line)
	}
	if t == "" {
		return fmt.Errorf("no token given")
	}

	client := cloudflare.New(t, "")
	if _, err := client.VerifyToken(ctx); err != nil {
		return fmt.Errorf("token rejected by Cloudflare: %w", err)
	}

	zones, err := client.Zones(ctx, "")
	if err != nil {
		return fmt.Errorf("list domains: %w", err)
	}
	if len(zones) == 0 {
		return fmt.Errorf("token verified, but it can reach no domains - check the Zone / DNS permission")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("read settings: %w", err)
	}
	cfg.APIToken = t

	switch {
	case *domain != "":
		cfg.Domain = *domain
	case cfg.Domain == "":
		cfg.Domain = zones[0].Name
	}

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("save settings: %w", err)
	}
	notifyTrayReload(ctx)

	path, _ := config.Path()
	fmt.Printf("Token verified. %d domain(s) available, default is %s.\n", len(zones), cfg.Domain)
	fmt.Printf("Saved to %s\n", path)
	if !config.SecretsAreEncrypted() {
		// Say it plainly. This is an account-wide credential that can
		// rewrite DNS for every zone on it, and on this platform the only
		// thing protecting it is file permissions.
		fmt.Fprintln(os.Stderr,
			"\nwarning: this platform has no OS keystore wired up yet, so the token is\n"+
				"stored in plain text with 0600 permissions. Any process running as you can read it.")
	}
	return nil
}

// ---------- domains ----------

func cmdDomains(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("domains", flag.ExitOnError)
	fs.Parse(args)

	s, err := open()
	if err != nil {
		return err
	}
	zones, err := s.client.Zones(ctx, "")
	if err != nil {
		return fmt.Errorf("list domains: %w", err)
	}

	w := out()
	fmt.Fprintln(w, "DOMAIN\tSTATUS\t")
	for _, z := range zones {
		mark := ""
		if strings.EqualFold(z.Name, s.cfg.Domain) {
			mark = "  (default)"
		}
		fmt.Fprintf(w, "%s\t%s%s\t\n", z.Name, z.Status, mark)
	}
	return w.Flush()
}

// ---------- route ----------

func cmdRoute(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("route needs a subcommand: add, ls, rm")
	}
	switch args[0] {
	case "add":
		return routeAdd(ctx, args[1:])
	case "ls", "list":
		return routeList(ctx, args[1:])
	case "rm", "remove":
		return routeRemove(ctx, args[1:])
	default:
		return fmt.Errorf("unknown route subcommand %q", args[0])
	}
}

func routeAdd(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("route add", flag.ExitOnError)
	port := fs.Int("port", 0, "local port to publish (required)")
	domain := fs.String("domain", "", "domain to publish under (default: the one from login)")
	pos := parseFlags(fs, args)

	if len(pos) != 1 {
		return fmt.Errorf("usage: quickflare route add <subdomain> --port <n>")
	}
	label := pos[0]
	if err := core.ValidateLabel(label); err != nil {
		return err
	}
	if _, err := core.ParsePort(fmt.Sprint(*port)); err != nil {
		return err
	}

	s, err := open()
	if err != nil {
		return err
	}
	z, err := s.zone(ctx, *domain)
	if err != nil {
		return err
	}
	if err := s.withTunnel(ctx); err != nil {
		return err
	}

	host := hostname(label, z.Name)
	target := fmt.Sprintf("localhost:%d", *port)

	routes := s.routes()
	for _, r := range routes {
		if strings.EqualFold(r.Hostname, host) {
			return fmt.Errorf("%s is already published", host)
		}
	}

	fmt.Printf("Publishing %s -> %s ...\n", host, target)
	if err := core.Publish(ctx, s.client, z.ID, s.tunnelID, host, target); err != nil {
		return err
	}

	routes = append(routes, core.Route{Hostname: host, Target: target, ZoneID: z.ID})
	if err := s.saveRoutes(routes); err != nil {
		return err
	}
	notifyTrayReload(ctx)

	fmt.Printf("https://%s is live.\n", host)
	fmt.Println("\nIt is public - QuickFlare adds no login. It answers only while a")
	fmt.Println("connector is running: 'quickflare run', or the systemd unit.")
	return nil
}

func routeList(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("route ls", flag.ExitOnError)
	check := fs.Bool("check", false, "verify each route against Cloudflare (slower)")
	fs.Parse(args)

	s, err := open()
	if err != nil {
		return err
	}
	routes := s.routes()
	if len(routes) == 0 {
		fmt.Println("No routes. Add one with 'quickflare route add <name> --port <n>'.")
		return nil
	}

	states := map[string]*core.State{}
	if *check {
		if err := s.withTunnel(ctx); err != nil {
			return err
		}
		z, err := s.zone(ctx, "")
		if err != nil {
			return err
		}
		states, err = core.FetchStates(ctx, s.client, s.tunnelID, z.ID, routes)
		if err != nil {
			return fmt.Errorf("check against Cloudflare: %w", err)
		}
	}

	sort.Slice(routes, func(i, j int) bool { return routes[i].Hostname < routes[j].Hostname })

	w := out()
	if *check {
		fmt.Fprintln(w, "HOSTNAME\tTARGET\tDNS\tINGRESS\t")
	} else {
		fmt.Fprintln(w, "HOSTNAME\tTARGET\t")
	}
	for _, r := range routes {
		if !*check {
			fmt.Fprintf(w, "%s\t%s\t\n", r.Hostname, r.Target)
			continue
		}
		st := states[strings.ToLower(r.Hostname)]
		if st == nil {
			st = &core.State{}
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t\n", r.Hostname, r.Target, yesNo(st.InDNS), yesNo(st.InIngress))
	}
	return w.Flush()
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func routeRemove(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("route rm", flag.ExitOnError)
	yes := fs.Bool("yes", false, "skip the confirmation prompt")
	pos := parseFlags(fs, args)

	if len(pos) != 1 {
		return fmt.Errorf("usage: quickflare route rm <hostname>")
	}
	host := strings.ToLower(pos[0])

	s, err := open()
	if err != nil {
		return err
	}

	routes := s.routes()
	idx := -1
	for i, r := range routes {
		if strings.EqualFold(r.Hostname, host) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("%s is not in the route list", host)
	}

	if !*yes {
		// Name what is about to be deleted. These are account-level objects,
		// not a row in a local list.
		fmt.Printf("Remove %s?\n", routes[idx].Hostname)
		fmt.Println("  - its DNS record, if QuickFlare created it")
		fmt.Println("  - its route in the tunnel")
		fmt.Print("Type y to confirm: ")
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if strings.ToLower(strings.TrimSpace(line)) != "y" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	if err := s.withTunnel(ctx); err != nil {
		return err
	}

	problems := core.Unpublish(ctx, s.client, routes[idx].ZoneID, s.tunnelID, routes[idx].Hostname)

	routes = append(routes[:idx], routes[idx+1:]...)
	if err := s.saveRoutes(routes); err != nil {
		return err
	}
	notifyTrayReload(ctx)

	fmt.Printf("Removed %s.\n", host)
	for _, p := range problems {
		fmt.Fprintln(os.Stderr, "  leftover: "+p)
	}
	return nil
}

// ---------- reconcile ----------

func cmdReconcile(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("reconcile", flag.ExitOnError)
	fs.Parse(args)

	s, err := open()
	if err != nil {
		return err
	}
	if err := s.withTunnel(ctx); err != nil {
		return err
	}
	z, err := s.zone(ctx, "")
	if err != nil {
		return err
	}

	stored := s.routes()
	states, err := core.FetchStates(ctx, s.client, s.tunnelID, z.ID, stored)
	if err != nil {
		return fmt.Errorf("read tunnel config: %w", err)
	}

	kept, dropped, adopted := core.ReconcileList(stored, states, z.ID)
	if err := s.saveRoutes(kept); err != nil {
		return err
	}

	for _, h := range dropped {
		fmt.Printf("dropped  %s  (gone from Cloudflare)\n", h)
	}
	for _, h := range adopted {
		fmt.Printf("adopted  %s  (served by the tunnel, not in the local list)\n", h)
	}

	// A half-published route is repaired rather than reported: one side is
	// missing, and re-publishing restores it.
	for _, r := range kept {
		if r.Status != core.StatusStarting {
			continue
		}
		fmt.Printf("repairing %s ...\n", r.Hostname)
		if err := core.Publish(ctx, s.client, r.ZoneID, s.tunnelID, r.Hostname, r.Target); err != nil {
			fmt.Fprintf(os.Stderr, "  failed: %v\n", err)
		}
	}

	fmt.Printf("%d route(s) in sync.\n", len(kept))
	return nil
}

// ---------- quick ----------

func cmdQuick(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("quick", flag.ExitOnError)
	port := fs.Int("port", 0, "local port to share (required)")
	fs.Parse(args)

	if _, err := core.ParsePort(fmt.Sprint(*port)); err != nil {
		return err
	}

	bin, err := supervisor.FindBinary()
	if err != nil {
		return err
	}

	q, err := supervisor.StartQuick(ctx, bin, *port)
	if err != nil {
		return err
	}
	defer q.Close()

	fmt.Fprintf(os.Stderr, "Requesting a link for localhost:%d ...\n", *port)

	// Poll rather than block forever: the URL arrives on cloudflared's
	// stderr a second or two in, and a tunnel that never produces one should
	// say so rather than hang.
	deadline := time.After(supervisor.QuickWaitTimeout)
	tick := time.NewTicker(150 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-q.Done():
			if e := q.Err(); e != nil {
				return e
			}
			return fmt.Errorf("tunnel stopped before it produced a link")
		case <-deadline:
			return fmt.Errorf("no link after %s", supervisor.QuickWaitTimeout)
		case <-tick.C:
			if u := q.URL(); u != "" {
				fmt.Println(u)
				fmt.Fprintln(os.Stderr, "\nAnyone with this link can open it. Ctrl-C ends it.")
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-q.Done():
					return q.Err()
				}
			}
		}
	}
}

// ---------- run ----------

func cmdRun(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	fs.Parse(args)

	s, err := open()
	if err != nil {
		return err
	}
	if err := s.withTunnel(ctx); err != nil {
		return err
	}

	token, err := s.client.TunnelToken(ctx, s.tunnelID)
	if err != nil {
		return fmt.Errorf("fetch tunnel token: %w", err)
	}

	bin, err := supervisor.FindBinary()
	if err != nil {
		return err
	}

	tun := supervisor.NewTunnel(bin)
	tun.OnStatus(func(st supervisor.Status) {
		if st.Err != nil {
			fmt.Fprintf(os.Stderr, "connector: %s (%v)\n", st.State, st.Err)
			return
		}
		fmt.Fprintf(os.Stderr, "connector: %s", st.State)
		if st.Connections > 0 {
			fmt.Fprintf(os.Stderr, " (%d connections)", st.Connections)
		}
		fmt.Fprintln(os.Stderr)
	})

	if err := tun.Start(ctx, token); err != nil {
		return err
	}
	defer tun.Stop()

	fmt.Fprintf(os.Stderr, "Serving %d route(s). Ctrl-C to stop.\n", len(s.cfg.Routes))
	<-ctx.Done()
	return nil
}

// ---------- status ----------

func cmdStatus(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	fs.Parse(args)

	path, _ := config.Path()
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("read settings: %w", err)
	}

	w := out()
	fmt.Fprintf(w, "config\t%s\t\n", path)

	client, _ := ipc.Discover()
	if client != nil {
		st, err := client.Status(ctx)
		if err == nil && st != nil {
			fmt.Fprintf(w, "tray app\trunning (PID %d)\t\n", st.PID)
			if st.Disabled {
				fmt.Fprintf(w, "route state\tpaused\t\n")
			} else {
				fmt.Fprintf(w, "route state\tactive\t\n")
			}
		} else {
			fmt.Fprintf(w, "tray app\trunning (PID %d)\t\n", client.PID())
		}
	} else {
		fmt.Fprintf(w, "tray app\tstopped\t\n")
		if cfg.RoutesDisabled {
			fmt.Fprintf(w, "route state\tpaused (saved)\t\n")
		} else {
			fmt.Fprintf(w, "route state\tactive (saved)\t\n")
		}
	}

	fmt.Fprintf(w, "token\t%s\t\n", tokenState(cfg))
	fmt.Fprintf(w, "token at rest\t%s\t\n", secretState())
	fmt.Fprintf(w, "domain\t%s\t\n", orNone(cfg.Domain))
	fmt.Fprintf(w, "routes\t%d cached\t\n", len(cfg.Routes))

	if bin, err := supervisor.FindBinary(); err == nil {
		fmt.Fprintf(w, "cloudflared\t%s\t\n", bin)
		if v := supervisor.EngineVersion(bin); v != "" {
			fmt.Fprintf(w, "engine\t%s\t\n", v)
		}
	} else {
		fmt.Fprintf(w, "cloudflared\tnot found: %v\t\n", err)
	}
	w.Flush()

	if cfg.APIToken == "" {
		fmt.Println("\nRun 'quickflare login' to get started.")
	}
	return nil
}

func tokenState(cfg *config.Config) string {
	if cfg.APIToken == "" {
		return "not set"
	}
	return "stored"
}

func secretState() string {
	if config.SecretsAreEncrypted() {
		return "encrypted by the OS keystore"
	}
	return "PLAIN TEXT, file permissions only"
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}
