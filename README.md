Thermite
========

[![Go Reference](https://pkg.go.dev/badge/github.com/dollarshaveclub/thermite.svg)](https://pkg.go.dev/github.com/dollarshaveclub/thermite)
[![CircleCI](https://circleci.com/gh/dollarshaveclub/thermite/tree/master.svg?style=svg)](https://circleci.com/gh/dollarshaveclub/thermite/tree/master)
[![Helm](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/dollarshaveclub)](https://artifacthub.io/packages/helm/dollarshaveclub/thermite)

## Installation

### go

	go get github.com/dollarshaveclub/thermite

### brew

	brew tap dollarshaveclub/homebrew-public
	brew install thermite

### docker

	docker pull dollarshaveclub/thermite

## thermite

Remove old and undeployed Amazon Elastic Container Registry images

### Synopsis

Thermite removes old Amazon Elastic Container Registry images that are not
deployed in a Kubernetes cluster.

Thermite checks for a resource tag (thermite:prune-period by default) on each
repository in an Elastic Container Registry. This tag specifies the number of
days that must pass after an image in the repository has been pushed before
is pruned.

Thermite surveys the image names of the containers associated with every
CronJob, DaemonSet, Deployment, Job, and StatefulSet in a Kubernetes
cluster, and excludes these images from removal.

Thermite expects shared environment configuration and credentials to exist for
the AWS account whose default Elastic Container Registry is to be pruned, as
described by the "Configuration and credentials" subsection of the "Configuring
the AWS CLI" section of the AWS Command Line Interface User Guide.

If Thermite is not running inside the Kubernetes cluster that is to be surveyed,
Thermite expects a Kubernetes configuration to exist as described in the
"Organizing Cluster Access Using kubeconfig Files" subsection of the
"Configuration" section of the Kubernetes Concepts documentation.

Thermite will submit DogStatsD metrics to the address specified by the
DD_AGENT_HOST and DD_DOGSTATSD_PORT environment variables if they are set.
Thermite will submit Datadog APM spans and profiles to the address specified by
the DD_AGENT_HOST and DD_TRACE_AGENT_PORT environment variables if they are set.

```
thermite [flags]
```

### Options

```
  -h, --help                      help for thermite
      --page-size uint            number of items returned in paginated API responses
      --period-tag-key string     AWS resource tag to check for prune period (default "thermite:prune-period")
  -y, --remove-images             enables removal of eligible images from ECR
      --statsd-namespace string   namespace to add to statsd metrics (default "thermite")
      --statsd-tag strings        tag to add to statsd metrics (supports multiple flags)
```

###### Auto generated by spf13/cobra on 20-Jul-2021
