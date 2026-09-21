package main

import (
	"fmt"
	"os"

	"github.com/smartattend/api/internal/auth"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: hashpwd <password>")
		os.Exit(2)
	}
	hash, err := auth.HashPassword(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(hash)
}
