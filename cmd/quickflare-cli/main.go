// Command quickflare-cli is QuickFlare's terminal interface.
//
// It exists for Linux, where there is no tray to rely on - GNOME does not
// implement StatusNotifierItem without an extension, and a box you are
// exposing a local port from is quite often a box you reached over SSH. On
// Windows the tray remains the only interface; this binary is not shipped
// there.
//
// It builds on every platform on purpose, so it can be developed and tested
// from the same machine as the tray app. Only packaging is Linux-only.
//
// There is no daemon and no IPC. QuickFlare's tunnel is remotely managed
// (config_src "cloudflare"), so adding and removing routes is pure API work
// that a one-shot process does correctly with no coordination - two
// invocations at once are as safe as two dashboard tabs. The only thing that
// must persist is the connector, which is "run".
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

const usageText = `quickflare - publish a localhost port through Cloudflare Tunnel

usage: quickflare <command> [flags]

  start               start the QuickFlare tray application
  stop                stop the running QuickFlare tray application
  pause               pause route forwarding (temporarily disable)
  resume              resume route forwarding (re-enable)
  login               store and verify a Cloudflare API token
  domains             list the zones the token can reach
  route add <name>    publish <name>.<domain> -> a local port
  route ls            list published routes
  route rm <host>     unpublish a route
  quick               a temporary trycloudflare.com link, no account needed
  run                 run the tunnel connector in the foreground
  service             install the connector as a systemd user service (Linux)
  status              token, domain, connector, route, and tray state
  reconcile           re-check stored routes against Cloudflare
  path                install or manage quickflare in Windows PATH
  uninstall           uninstall QuickFlare from this machine

Run 'quickflare <command> -h' for the flags a command takes.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usageText)
		os.Exit(2)
	}

	// One context for the whole run, cancelled on Ctrl-C. Long-running
	// commands ("run", "quick") need it; the short ones get tidy cancellation
	// for free rather than leaving a half-finished API call behind.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	args := os.Args[2:]
	var err error

	switch os.Args[1] {
	case "start":
		err = cmdStart(ctx, args)
	case "stop":
		err = cmdStop(ctx, args)
	case "pause":
		err = cmdPause(ctx, args)
	case "resume":
		err = cmdResume(ctx, args)
	case "login":
		err = cmdLogin(ctx, args)
	case "domains":
		err = cmdDomains(ctx, args)
	case "route":
		err = cmdRoute(ctx, args)
	case "quick":
		err = cmdQuick(ctx, args)
	case "run":
		err = cmdRun(ctx, args)
	case "service":
		err = cmdService(ctx, args)
	case "status":
		err = cmdStatus(ctx, args)
	case "reconcile":
		err = cmdReconcile(ctx, args)
	case "path":
		err = cmdPath(ctx, args)
	case "uninstall":
		err = cmdUninstall(ctx, args)
	case "help", "-h", "--help":
		fmt.Print(usageText)
		return
	default:
		fmt.Fprintf(os.Stderr, "quickflare: unknown command %q\n\n", os.Args[1])
		fmt.Fprint(os.Stderr, usageText)
		os.Exit(2)
	}

	if err != nil {
		// A cancelled context is Ctrl-C, not a failure. Reporting it as an
		// error would make every interrupted "quick" look like a crash.
		if errors.Is(err, context.Canceled) {
			return
		}
		fmt.Fprintln(os.Stderr, "quickflare: "+err.Error())
		os.Exit(1)
	}
}
