package main

import (
	"log"

	"git-worktree-tui/internal/tui"
)

func main() {
	p := tui.NewProgram()
	if err := p.Start(); err != nil {
		log.Fatal(err)
	}
}
