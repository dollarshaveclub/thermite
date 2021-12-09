// Package prune removes images from Amazon Web Services Elastic Container
// Registry repositories based on their age.
package prune

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"strconv"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/aws/aws-sdk-go/service/ecr/ecriface"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

// A GarbageCollector removes images from Amazon ECR based on age.
type GarbageCollector interface {
	PruneRepo(ctx context.Context, name string, until time.Time, excluded ...string) (pruned []string, err error)
	PruneAllRepos(ctx context.Context, until time.Time, excluded ...string) (pruned []string, err error)
}

type whitelist map[string]struct{}

func newWhitelist(imageRefs ...string) whitelist {
	wl := make(whitelist, len(imageRefs))
	for _, ref := range imageRefs {
		wl[ref] = struct{}{}
	}
	return wl
}

func (wl whitelist) IsExcluded(imageRef string) bool {
	_, ok := wl[imageRef]
	return ok
}

// A Client is a configurable GarbageCollector wrapping ecriface.ECRAPI.
type Client struct {
	client              ecriface.ECRAPI
	periodTagKey        string
	pageSize            uint
	removeImages        bool
	allowZeroExclusions bool
	logger              *log.Logger
	statsd              statsd.ClientInterface
}

// An Option is an option applied when creating a Client.
type Option func(gc *Client)

// WithRemoveImages sets whether a Client should remove images from Elastic
// Container Registry or just determine which images are eligible.
func WithRemoveImages() Option {
	return func(gc *Client) {
		gc.removeImages = true
	}
}

// WithAllowZeroExclusions will allow a Client to prune even if no images are
// excluded.
func WithAllowZeroExclusions() Option {
	return func(gc *Client) {
		gc.allowZeroExclusions = true
	}
}

// WithPeriodTagKey sets the Amazon Web Services resource tag used to specify
// ECR repository prune periods to a Client.
func WithPeriodTagKey(key string) Option {
	return func(gc *Client) {
		gc.periodTagKey = key
	}
}

// DefaultPeriodTagKey is the default Amazon Web Services resource tag used to
// specify Elastic Container Registry repository prune periods to a Client.
const DefaultPeriodTagKey = "thermite:prune-period"

// PeriodTagKey returns the resource tag used to specify Elastic Container
// Registry repository prune periods to gc.
func (gc *Client) PeriodTagKey() string {
	return gc.periodTagKey
}

// WithPageSize sets the maximum number of responses a Client should request
// in a single Elastic Container Registry API call.
func WithPageSize(size uint) Option {
	return func(gc *Client) {
		gc.pageSize = size
	}
}

// WithLogger sets a logger for a Client to output to.
func WithLogger(logger *log.Logger) Option {
	return func(c *Client) {
		c.logger = logger
	}
}

// WithStatsdClient sets a statsd client to use to report metrics from a Client.
func WithStatsdClient(client statsd.ClientInterface) Option {
	return func(c *Client) {
		c.statsd = client
	}
}

// NewClient returns a GarbageCollector that removes images using
// client. If no WithPeriodTagKey options are specified in opts,
// DefaultPeriodTagKey will be used.
func NewClient(client ecriface.ECRAPI, opts ...Option) (*Client, error) {
	if client == nil {
		return nil, fmt.Errorf("client must not be nil")
	}
	gc := &Client{
		client:       client,
		periodTagKey: DefaultPeriodTagKey,
		logger:       log.New(io.Discard, "", 0),
		statsd:       &statsd.NoOpClient{},
	}
	for _, opt := range opts {
		opt(gc)
	}
	return gc, nil
}

// PruneAllRepos runs PruneRepo for every repository in the Amazon Elastic
// Container Registry associated with gc, and returns the combined list
// of pruned image references.
func (gc *Client) PruneAllRepos(ctx context.Context, until time.Time, excluded ...string) (pruned []string, err error) {
	var span tracer.Span
	span, ctx = tracer.StartSpanFromContext(ctx, "prune.Client.PruneAllRepos")
	defer span.Finish()
	defer gc.statsd.Flush()
	pruned = []string{}
	dro, err := gc.client.DescribeRepositoriesWithContext(ctx, &ecr.DescribeRepositoriesInput{
		MaxResults: gc.maxResults(),
	})
	if err != nil {
		span.Finish(tracer.WithError(err))
		return pruned, fmt.Errorf("error describing Elastic Container Registry repositories: %w", err)
	}
	taggedRepoCount := 0
	for _, repo := range dro.Repositories {
		repoPruned, err := gc.PruneRepo(ctx, *repo.RepositoryName, until, excluded...)
		pruned = append(pruned, repoPruned...)
		if err != nil && err != ErrNoPrunePeriodTag {
			span.Finish(tracer.WithError(err))
			return pruned, fmt.Errorf("error pruning repository %s: %w", *repo.RepositoryUri, err)
		}
		if err == nil {
			taggedRepoCount++
		}
	}
	gc.logger.Printf("pruned %d Elastic Container Registry images", len(pruned))
	gc.statsd.Gauge("prune.tagged_repos", float64(taggedRepoCount), nil, 1)
	gc.statsd.Gauge("prune.prune_all_repos", float64(len(dro.Repositories)), nil, 1)
	return pruned, nil
}

var ErrNoPrunePeriodTag = errors.New("no valid prune period tag for repository")

// PruneRepo checks the named repo for a tag with the key identified by
// gc.PeriodTagKey(), whose value specifies a positive integer representing the
// number of days that must pass after an image is pushed to the repository
// before it can be removed. If the tag is present, PruneRepo removes any images
// that were pushed that many days before until, excluding any image referenced
// by excluded.
//
// PruneRepo returns the list of image references that were pruned (or would
// haveb been pruned if WithRemoveImages was not specified as an option when
// creating gc). PruneRepo will fail if no image references are specified by
// excluded, unless WithAllowZeroExclusions was specified when creating gc.
func (gc *Client) PruneRepo(ctx context.Context, name string, until time.Time, excluded ...string) (pruned []string, err error) {
	var span tracer.Span
	span, ctx = tracer.StartSpanFromContext(ctx, "prune.Client.PruneRepo")
	defer span.Finish()
	defer gc.statsd.Flush()
	pruned = []string{}
	if len(excluded) == 0 && !gc.allowZeroExclusions {
		return pruned, fmt.Errorf("zero images excluded from prune")
	}
	repo, err := gc.repoFromName(ctx, name)
	if err != nil {
		span.Finish(tracer.WithError(err))
		return pruned, fmt.Errorf("error looking up repository: %w", err)
	}
	period, ok, err := gc.repoPrunePeriodFromARN(ctx, *repo.RepositoryArn)
	if err != nil {
		span.Finish(tracer.WithError(err))
		return pruned, fmt.Errorf("error checking for prune period: %w", err)
	}
	if !ok {
		return pruned, ErrNoPrunePeriodTag
	}
	log.Printf(
		"found prune period of %d days for Elastic Container Registry repository %s",
		period,
		name,
	)
	imageDetails := make([]*ecr.ImageDetail, 0)
	var mostRecentImageDetail *ecr.ImageDetail
	if err := gc.client.DescribeImagesPagesWithContext(
		ctx,
		&ecr.DescribeImagesInput{
			RepositoryName: aws.String(name),
			MaxResults:     gc.maxResults(),
		},
		func(page *ecr.DescribeImagesOutput, lastPage bool) bool {
			for _, imageDetail := range page.ImageDetails {
				isFirst := mostRecentImageDetail == nil
				isMostRecent := isFirst || imageDetail.ImagePushedAt.After(*mostRecentImageDetail.ImagePushedAt)
				if isMostRecent {
					mostRecentImageDetail = imageDetail
				}
			}
			imageDetails = append(imageDetails, page.ImageDetails...)
			return true
		},
	); err != nil {
		span.Finish(tracer.WithError(err))
		return pruned, fmt.Errorf(
			"error describing images in Elastic Container Registry repository %s: %w",
			name,
			err,
		)
	}
	if mostRecentImageDetail != nil {
		log.Println("*****")
		mostRecentImageIDs := make([]*ecr.ImageIdentifier, 0, len(mostRecentImageDetail.ImageTags))
		for _, imageTag := range mostRecentImageDetail.ImageTags {
			imageID := &ecr.ImageIdentifier{ImageTag: imageTag}
			mostRecentImageIDs = append(mostRecentImageIDs, imageID)
		}
		mostRecentImageRefs, err := repoImageRefsFromURIAndImageIDs(
			ctx,
			*repo.RepositoryUri,
			mostRecentImageIDs,
		)
		if err != nil {
			span.Finish(tracer.WithError(err))
			return nil, err
		}
		excluded = append(excluded, mostRecentImageRefs...)
	}
	whitelist := newWhitelist(excluded...)
	log.Println(whitelist)
	pruneableImageIDs := make([]*ecr.ImageIdentifier, 0, len(imageDetails))
excluded:
	for _, imageDetail := range imageDetails {
		if imageDetail.ImagePushedAt == nil {
			span.Finish(tracer.WithError(err))
			return nil, fmt.Errorf(
				"found unexpected nil image pushed at time in Elastic Container Registry repository %s",
				*repo.RepositoryUri,
			)
		}
		pushedAt := imageDetail.ImagePushedAt.UTC()
		cutoff := until.UTC().Add(-time.Duration(period) * 24 * time.Hour)
		if pushedAt.After(cutoff) {
			continue excluded
		}
		for _, imageTag := range imageDetail.ImageTags {
			if imageTag == nil {
				span.Finish(tracer.WithError(err))
				return nil, fmt.Errorf(
					"found unexpected nil image tag in Elastic Container Registry repository %s",
					*repo.RepositoryUri,
				)
			}
			imageRef := fmt.Sprintf("%s:%s", *repo.RepositoryUri, *imageTag)
			if whitelist.IsExcluded(imageRef) {
				continue excluded
			}
			log.Println(imageRef, "is prunable")
			imageID := &ecr.ImageIdentifier{ImageTag: imageTag}
			pruneableImageIDs = append(pruneableImageIDs, imageID)
		}
	}
	log.Printf(
		"found %d unique pruneable images for Elastic Container Registry repository %s",
		len(pruneableImageIDs),
		name,
	)
	gc.statsd.Gauge("prune.prune_repo_pruneable", float64(len(pruneableImageIDs)), nil, 1)
	if !gc.removeImages {
		gc.statsd.Count("prune.prune_repo_deleted", 0, nil, 1)
		pruneableImageTags, err := repoImageRefsFromURIAndImageIDs(
			ctx,
			*repo.RepositoryUri,
			pruneableImageIDs,
		)
		if err != nil {
			return pruned, err
		}
		return pruneableImageTags, nil
	}
	pruned = make([]string, 0, len(pruneableImageIDs))
	remaining := pruneableImageIDs
	for len(remaining) > 0 {
		batch := remaining
		if len(batch) > 100 {
			batch = batch[:100]
		}
		remaining = remaining[len(batch):]
		bdio, batchDeleteImageErr := gc.client.BatchDeleteImageWithContext(
			ctx,
			&ecr.BatchDeleteImageInput{
				ImageIds:       batch,
				RepositoryName: repo.RepositoryName,
			},
		)
		log.Printf(
			"deleted %d images from Elastic Container Registry repository %s",
			len(bdio.ImageIds),
			name,
		)
		gc.statsd.Count("prune.prune_repo_deleted", int64(len(bdio.ImageIds)), nil, 1)
		deletedImageRefs, err := repoImageRefsFromURIAndImageIDs(ctx, *repo.RepositoryUri, bdio.ImageIds)
		if err != nil {
			span.Finish(tracer.WithError(err))
			return pruned, fmt.Errorf("error formatting deleted image names: %w", err)
		}
		pruned = append(pruned, deletedImageRefs...)
		if batchDeleteImageErr != nil {
			span.Finish(tracer.WithError(batchDeleteImageErr))
			return pruned, fmt.Errorf("error deleting images: %w", batchDeleteImageErr)
		}

	}
	return pruned, nil
}

func (gc *Client) repoFromName(ctx context.Context, name string) (*ecr.Repository, error) {
	var span tracer.Span
	span, ctx = tracer.StartSpanFromContext(ctx, "prune.Client.repoFromName")
	defer span.Finish()
	dro, err := gc.client.DescribeRepositoriesWithContext(ctx, &ecr.DescribeRepositoriesInput{
		RepositoryNames: []*string{aws.String(name)},
	})
	if err != nil {
		span.Finish(tracer.WithError(err))
		return nil, fmt.Errorf("error describing repository: %w", err)
	}
	if len(dro.Repositories) < 1 {
		return nil, fmt.Errorf("no repositories found")
	}
	return dro.Repositories[0], nil
}

func (gc *Client) repoTagsFromARN(ctx context.Context, arn string) ([]*ecr.Tag, error) {
	var span tracer.Span
	span, ctx = tracer.StartSpanFromContext(ctx, "prune.Client.repoTagsFromARN")
	defer span.Finish()
	ltfro, err := gc.client.ListTagsForResourceWithContext(ctx, &ecr.ListTagsForResourceInput{
		ResourceArn: aws.String(arn),
	})
	if err != nil {
		span.Finish(tracer.WithError(err))
		return nil, fmt.Errorf("error listing tags: %w", err)
	}
	return ltfro.Tags, nil
}

func (gc *Client) repoPrunePeriodFromARN(
	ctx context.Context,
	arn string,
) (period int, ok bool, err error) {
	var span tracer.Span
	span, ctx = tracer.StartSpanFromContext(ctx, "prune.Client.repoPrunePeriodFromARN")
	defer span.Finish()
	tags, err := gc.repoTagsFromARN(ctx, arn)
	if err != nil {
		span.Finish(tracer.WithError(err))
		return 0, false, fmt.Errorf("error looking up tags: %w", err)
	}
	period, ok = 0, false
	for _, tag := range tags {
		if tag.Key == nil || *tag.Key != gc.PeriodTagKey() {
			continue
		}
		if tag.Value == nil {
			log.Printf("prune period tag key %s for %s has nil value", *tag.Key, arn)
			break
		}
		period64, err := strconv.ParseUint(*tag.Value, 10, 0)
		if err != nil {
			log.Printf("prune period tag value %s for %s is not parseable as an unsigned integer", *tag.Value, arn)
			break
		}
		if period64 == 0 {
			log.Printf("prune period for %s is zero", arn)
			break
		}
		period, ok = int(period64), true
	}
	return period, ok, nil
}

func (gc *Client) maxResults() *int64 {
	maxResults := int64(gc.pageSize)
	if maxResults == 0 {
		return nil
	}
	return aws.Int64(maxResults)
}

func repoImageRefsFromURIAndImageIDs(ctx context.Context, uri string, imageIDs []*ecr.ImageIdentifier) ([]string, error) {
	imageRefs := make([]string, 0, len(imageIDs))
	for _, imageID := range imageIDs {
		if imageID.ImageTag == nil {
			return nil, fmt.Errorf("imageID.ImageTag must not be nil")
		}
		imageRef := fmt.Sprintf("%s:%s", uri, *imageID.ImageTag)
		imageRefs = append(imageRefs, imageRef)
	}
	return imageRefs, nil
}
