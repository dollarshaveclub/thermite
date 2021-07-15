// +build doc

package main

import (
	"os"

	"github.com/dollarshaveclub/thermite/cmd"
	"github.com/spf13/cobra/doc"

	_ "embed"
)

//go:embed README.md
var fm string

func main() {
	file, err := os.Create("README.md")
	if err != nil {
		panic(err)
	}
	defer file.Close()
	if _, err := file.Write([]byte(fm + "\n")); err != nil {
		panic(err)
	}
	if err := doc.GenMarkdown(
		cmd.RootCmd,
		file,
	); err != nil {
		panic(err)
	}
}
