package main

import (
	"fmt"
	"os"

	"customer-support/internal/handler"
)

func main() {
	if err := handler.BuildQuoteFormTemplates("", ""); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
