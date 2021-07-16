Thermite
========

[![Go Reference](https://pkg.go.dev/badge/github.com/dollarshaveclub/thermite.svg)](https://pkg.go.dev/github.com/dollarshaveclub/thermite)
[![CircleCI](https://circleci.com/gh/dollarshaveclub/thermite/tree/master.svg?style=svg)](https://circleci.com/gh/dollarshaveclub/thermite/tree/master)

## Installation

### go

	go get github.com/dollarshaveclub/thermite

### brew

	brew tap dollarshaveclub/homebrew-public
	brew install thermite

### docker

	docker pull dollarshaveclub/thermite

## Deployment

    helm repo add dollarshaveclub https://dollarshaveclub.github.io/helm-charts-public
	helm repo update
	helm install my-release dollarshaveclub/thermite

