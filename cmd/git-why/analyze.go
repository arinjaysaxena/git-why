package main

import (
	"errors"
	"io"
)

// errNotWired is returned until the history pipeline is connected to the command.
var errNotWired = errors.New("history analysis is not available in this build yet")

func analyze(_ io.Writer, _ *options) error { return errNotWired }
