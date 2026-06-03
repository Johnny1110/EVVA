package main

import "fmt"

// runSwarm dispatches the swarm control-plane CLI:
//
//	evva swarm .            register the current workdir's evva-swarm.yml into
//	                        the running service as a new (isolated) space
//	evva swarm ls           list registered spaces
//	evva swarm stop <name>  stop one space
//	evva swarm add <name>   hot-load a new member into a space
//
// At M0 (SPRD-1-1) these parse and route only; the real implementations (POST
// the workdir to the running service, etc.) land in SPRD-1-9.
func runSwarm(args []string) {
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}

	switch sub {
	case "":
		exitf(2, "usage: evva swarm . | ls | stop <name> | add <name>")
	case ".":
		fmt.Println("evva swarm .: not implemented yet (SPRD-1-9: register ./evva-swarm.yml into the running service)")
	case "ls":
		fmt.Println("evva swarm ls: not implemented yet (SPRD-1-9)")
	case "stop":
		fmt.Println("evva swarm stop: not implemented yet (SPRD-1-9)")
	case "add":
		fmt.Println("evva swarm add: not implemented yet (SPRD-1-9)")
	default:
		exitf(2, "evva swarm: unknown subcommand %q (want . | ls | stop | add)", sub)
	}
}
