Thermite
========

Thermite removes old Amazon Elastic Container Registry images that are not
deployed in a Kubernetes cluster

## Introduction

This chart bootstraps a Thermite deployment on a Kubernetes cluster using the
Helm package manager.

## Installing the chart

To install the chart with the release name `thermite` run:

    helm repo add dollarshaveclub
    helm repo add dollarshaveclub https://dollarshaveclub.github.io/helm-charts-public

The command deploys Thermite  on the Kubernetes cluster using the default
configuration. The configuration section lists the parameters that can be
configured during installation.

## Uninstallng the chart

To uninstall the `thermite` deployment run:

    helm uninstall thermite

## Configuration

Please see the values schema reference documentation in Artifact Hub for a list
of the configurable parameters of the chart and their default values. Specify
each parameter using the `--set key=value[,key=value]` argument to helm install.