package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"github.com/alpindale/ssh-dashboard/internal"
	tea "github.com/charmbracelet/bubbletea"
)

func validateInterval(seconds float64) time.Duration {
	if seconds < 0.01 || seconds > 3600 {
		return 0
	}
	return time.Duration(seconds * float64(time.Second))
}

func main() {
	flag.Usage = func() {
		// HACK: make it look like python's argparse
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		fmt.Fprintf(os.Stderr, "  -n, --interval float  Update interval in seconds (default: 5, or SSH_DASHBOARD_INTERVAL env var)\n")
		fmt.Fprintf(os.Stderr, "  -h, --help            Show this help message\n")
	}

	var updateIntervalVal float64
	flag.Float64Var(&updateIntervalVal, "n", 0, "")
	flag.Float64Var(&updateIntervalVal, "interval", 0, "")
	flag.Parse()
	updateInterval := &updateIntervalVal

	interval := 5 * time.Second

	if *updateInterval > 0 {
		if validated := validateInterval(*updateInterval); validated > 0 {
			interval = validated
		}
	} else if envInterval := os.Getenv("SSH_DASHBOARD_INTERVAL"); envInterval != "" {
		if seconds, err := strconv.ParseFloat(envInterval, 64); err == nil {
			if validated := validateInterval(seconds); validated > 0 {
				interval = validated
			}
		}
	}

	hosts, err := internal.ParseSSHConfig("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing SSH config: %v\n", err)
		os.Exit(1)
	}

	if len(hosts) == 0 {
		fmt.Fprintf(os.Stderr, "No hosts found in SSH config\n")
		os.Exit(1)
	}

	initialModel := internal.InitialModel(hosts, interval)
	p := tea.NewProgram(initialModel, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running program: %v\n", err)
		os.Exit(1)
	}

	if m, ok := finalModel.(internal.Model); ok {
		if sshHost := m.GetSSHOnExit(); sshHost != "" {
			sshPath, err := exec.LookPath("ssh")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error finding ssh: %v\n", err)
				os.Exit(1)
			}

			args := []string{"ssh", sshHost}
			env := os.Environ()

			err = syscall.Exec(sshPath, args, env)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error executing ssh: %v\n", err)
				os.Exit(1)
			}
		}
	}
}
