package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/Jaro-c/Lynx/internal/ipc"
	"github.com/Jaro-c/Lynx/internal/term"
	"github.com/Jaro-c/Lynx/internal/types"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: lynx <command>")
		os.Exit(1)
	}

	command := os.Args[1]

	// Common client setup
	client, err := ipc.NewClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", term.RedString("Failed to connect to daemon: %v", err))
		os.Exit(1)
	}
	defer client.Close()

	switch command {
	case "ping":
		var result map[string]string
		if err := client.Call("ping", nil, &result); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", term.RedString("Ping failed: %v", err))
			os.Exit(1)
		}
		fmt.Printf("%s %s\n", term.GreenString("Success"), term.BoldString("pong"))

	case "status":
		var processes []types.ProcessInfo
		if err := client.Call("status", nil, &processes); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", term.RedString("Status failed: %v", err))
			os.Exit(1)
		}
		renderStatus(processes)

	default:
		fmt.Printf("%s\n", term.YellowString("Unknown command: %s", command))
		os.Exit(1)
	}
}

func renderStatus(processes []types.ProcessInfo) {
	if len(processes) == 0 {
		fmt.Println("No processes managed.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATE\tPID\tUPTIME\tCPU\tMEM")

	for _, p := range processes {
		stateStr := p.State
		var coloredState string

		switch p.State {
		case types.StateRunning:
			coloredState = term.GreenString(string(stateStr))
		case types.StateStopped:
			coloredState = term.RedString(string(stateStr))
		case types.StateFailed:
			coloredState = term.RedString(string(stateStr))
		default:
			coloredState = string(stateStr)
		}

		pidStr := "-"
		if p.PID > 0 {
			pidStr = fmt.Sprintf("%d", p.PID)
		}

		cpuStr := p.CPU
		if cpuStr == "" {
			cpuStr = term.DimString("-")
		}

		memStr := p.Memory
		if memStr == "" {
			memStr = term.DimString("-")
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			term.BoldString(p.Name),
			coloredState,
			pidStr,
			p.Uptime,
			cpuStr,
			memStr,
		)
	}
	w.Flush()
}
