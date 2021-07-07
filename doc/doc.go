// +build doc

package main

import (
	"os"

	"github.com/dollarshaveclub/thermite/cmd"
	"github.com/spf13/cobra/doc"
)

const fm = `Thermite
========

[![Go Reference](https://pkg.go.dev/badge/github.com/dollarshaveclub/thermite.svg)](https://pkg.go.dev/github.com/dollarshaveclub/thermite)
[![CircleCI](https://circleci.com/gh/dollarshaveclub/thermite/tree/master.svg?style=svg)](https://circleci.com/gh/dollarshaveclub/thermite/tree/master)

	go get github.com/dollarshaveclub/thermite

	brew tap dollarshaveclub/homebrew-public
	brew install thermite

	docker pull dollarshaveclub/thermite
`

func main() {
	file, err := os.Create("README.md")
	if err != nil {
		panic(err)
	}
	defer file.Close()
	if _, err := file.Write([]byte(fm)); err != nil {
		panic(err)
	}
	if err := doc.GenMarkdown(
		cmd.RootCmd,
		file,
	); err != nil {
		panic(err)
	}
}
