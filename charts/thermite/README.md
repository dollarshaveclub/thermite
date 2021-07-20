Thermite
========

Thermite removes old Amazon Elastic Container Registry images that are not
deployed in a Kubernetes cluster

## Introduction

This chart bootstraps a Thermite deployment on a Kubernetes cluster using the
Helm package manager.

## Installing the chart

To install the chart with the release name `thermite` run:

    helm repo add dollarshaveclub https://dollarshaveclub.github.io/helm-charts-public
    helm install thermite dollarshaveclub/thermite

The command deploys Thermite on the Kubernetes cluster using the default
configuration. The configuration section lists the parameters that can be
configured during installation.

## Removing images

The chart installs one CronJob in charge of running Thermite periodically (every
day at midnight based on the timezone of the kube-controller-manager), which
removes images from Amazon Elastic Container Registry repositories with a valid
prune period resource tag (`thermite:prune-period`) that were pushed longer ago
than the repository's prune period, excluding images that are currently deployed
in the Kubernetes cluster. If you don't want to wait until the job is triggered
by the cronjob, you can create one manually using the following command:

    kubectl create job initial-thermite-job --from=cronjob/my-release-thermite

By default, the chart will disable image removal and only log/report the images
that are eligible. To enable image removal, the `removeImages` parameter must be
set to `true`.

## Uninstalling the chart

To uninstall the `thermite` deployment run:

    helm uninstall thermite

## Configuration

Please see the values schema reference documentation in Artifact Hub for a list
of the configurable parameters of the chart and their default values. Specify
each parameter using the `--set key=value[,key=value]` argument to helm install.
For example,

    helm install thermite --set removeImages=true dollarshaveclub/thermite

Alternatively, a YAML file that specifies the values for the parameters can be provided while installing the chart. For example,

    helm install thermite -f values.yaml dollarshaveclub/thermite