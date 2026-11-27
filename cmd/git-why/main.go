// Command git-why explains why a line of code looks the way it does by
// reconstructing the decision history behind it. Installed as `git-why`, it
// also works as the git subcommand `git why`.
package main

import (
	"errors"
	"fmt"
	"os"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "git-why: %v\n", err)
		var ue *usageError
		if errors.As(err, &ue) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}
