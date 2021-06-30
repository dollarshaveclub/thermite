// Package thermite removes old Amazon Elastic Container Registry images that
// are not currently deployed in a Kubernetes cluster.
package thermite

import (
	"context"
	"fmt"
	"time"

	"github.com/dollarshaveclub/thermite/pkg/census"
	"github.com/dollarshaveclub/thermite/pkg/prune"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

// A Client removes old images from Amazon Elastic Container Registry that are
// not currently deployed in a Kubernetes cluster.
type Client struct {
	taker census.Taker
	gc    prune.GarbageCollector
}

// NewClient returns a Client that removes eligible images from ecr, excluding
// images currently deployed in kubernetes. If no WithPeriodTagKey options are
// specified in opts, DefaultPeriodTagKey will be used.
func NewClient(taker census.Taker, gc prune.GarbageCollector) (*Client, error) {
	if taker == nil {
		return nil, fmt.Errorf("taker must not be nil")
	}
	if gc == nil {
		return nil, fmt.Errorf("gc must not be nil")
	}
	c := &Client{
		taker: taker,
		gc:    gc,
	}
	return c, nil
}

// Run looks at every repository in the Amazon Elastic Container Registry
// associated with c, checks for an AWS resource tag on the repository that
// specifies a prune period (a positive integer representing the number of days
// that must pass after an image is pushed to the repository before it can be
// removed), and if the tag is present, removes any images that were pushed that
// many days before until. Run returns the list of image references that were
// pruned, along with any error that occurred.
func (c *Client) Run(ctx context.Context, until time.Time) (pruned []string, err error) {
	var span tracer.Span
	span, ctx = tracer.StartSpanFromContext(ctx, "thermite.Client.Run")
	defer span.Finish()
	surveyed, err := c.taker.SurveyDeployedImages(ctx)
	if err != nil {
		return nil, fmt.Errorf("error surveying Kubernetes images: %w", err)
	}
	pruned, err = c.gc.PruneAllRepos(ctx, until, surveyed...)
	if err != nil {
		return nil, fmt.Errorf("error pruning ECR images: %w", err)
	}
	return pruned, nil
}
