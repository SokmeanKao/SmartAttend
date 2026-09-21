package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/smartattend/api/internal/auth"
)

func main() {
	os.Exit(run(os.Args, os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: read password from stdin")
		return 2
	}

	password, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && err != io.EOF {
		fmt.Fprintln(stderr, "read password:", err)
		return 1
	}
	password = strings.TrimSuffix(password, "\n")
	password = strings.TrimSuffix(password, "\r")
	if password == "" {
		fmt.Fprintln(stderr, "password must not be empty")
		return 2
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, hash)
	return 0
}
