package prune

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/aws/aws-sdk-go/service/ecr/ecriface"
	"github.com/google/go-cmp/cmp"
)

type mockedClient struct {
	ecriface.ECRAPI
	Repositories                 []*ecr.Repository
	TagsByResourceARN            map[string][]*ecr.Tag
	ImageDetailsByRepositoryName map[string][]*ecr.ImageDetail
	deletedCount                 int
}

func (m mockedClient) DescribeRepositoriesWithContext(
	ctx aws.Context,
	input *ecr.DescribeRepositoriesInput,
	opts ...request.Option,
) (*ecr.DescribeRepositoriesOutput, error) {
	if opts != nil {
		return nil, fmt.Errorf("opts must be nil")
	}
	if input.NextToken != nil {
		return nil, fmt.Errorf("input.NextToken must be nil")
	}
	if input.RegistryId != nil {
		return nil, fmt.Errorf("input.RegistryId must be nil")
	}
	repositoriesByName := make(map[string]*ecr.Repository, len(m.Repositories))
	for _, repo := range m.Repositories {
		repositoriesByName[*repo.RepositoryName] = repo
	}
	repos := make([]*ecr.Repository, 0, len(input.RepositoryNames))
	for name, repo := range repositoriesByName {
		included := len(input.RepositoryNames) == 0
		for _, inputName := range input.RepositoryNames {
			if *inputName == name {
				included = true
				break
			}
		}
		if !included {
			continue
		}
		repos = append(repos, repo)
	}
	return &ecr.DescribeRepositoriesOutput{
		Repositories: repos,
	}, nil
}

func (m mockedClient) DescribeRepositoriesPagesWithContext(
	ctx aws.Context,
	input *ecr.DescribeRepositoriesInput,
	fn func(*ecr.DescribeRepositoriesOutput, bool) bool,
	opts ...request.Option,
) error {
	dro, err := m.DescribeRepositoriesWithContext(ctx, input)
	if err != nil {
		return err
	}
	repos := dro.Repositories
	if input.MaxResults != nil {
		for int64(len(repos)) > *input.MaxResults {
			repos = repos[:*input.MaxResults]
			if !fn(&ecr.DescribeRepositoriesOutput{
				Repositories: repos,
			}, false) {
				return nil
			}
		}
	}
	fn(&ecr.DescribeRepositoriesOutput{
		Repositories: repos,
	}, true)
	return nil
}

func (m mockedClient) ListTagsForResourceWithContext(
	ctx aws.Context,
	input *ecr.ListTagsForResourceInput,
	opts ...request.Option,
) (*ecr.ListTagsForResourceOutput, error) {
	if opts != nil {
		return nil, fmt.Errorf("opts must be nil")
	}
	if input.ResourceArn == nil {
		return nil, fmt.Errorf("input.ResourceARN must not be nil")
	}
	tags, ok := m.TagsByResourceARN[*input.ResourceArn]
	if !ok {
		return nil, fmt.Errorf("ARN %s not found", *input.ResourceArn)
	}
	return &ecr.ListTagsForResourceOutput{
		Tags: tags,
	}, nil
}

func (m mockedClient) DescribeImagesPagesWithContext(
	ctx aws.Context,
	input *ecr.DescribeImagesInput,
	fn func(*ecr.DescribeImagesOutput, bool) bool,
	opts ...request.Option,
) error {
	if opts != nil {
		return fmt.Errorf("opts must be nil")
	}
	if input.Filter != nil {
		return fmt.Errorf("input.Filter must be nil")
	}
	if input.ImageIds != nil {
		return fmt.Errorf("input.ImageIds must be nil")
	}
	if input.NextToken != nil {
		return fmt.Errorf("input.NextToken must be nil")
	}
	if input.RegistryId != nil {
		return fmt.Errorf("input.RegistryId must be nil")
	}
	if input.RepositoryName == nil {
		return fmt.Errorf("input.RepositoryName must not be nil")
	}
	imageDetails, ok := m.ImageDetailsByRepositoryName[*input.RepositoryName]
	if !ok {
		return fmt.Errorf("repository with name %s not found", *input.RepositoryName)
	}
	if input.MaxResults != nil {
		for int64(len(imageDetails)) > *input.MaxResults {
			imageDetails = imageDetails[:*input.MaxResults]
			if !fn(&ecr.DescribeImagesOutput{
				ImageDetails: imageDetails,
			}, false) {
				return nil
			}
		}
	}
	fn(&ecr.DescribeImagesOutput{
		ImageDetails: imageDetails,
	}, true)
	return nil
}

func (m *mockedClient) BatchDeleteImageWithContext(
	ctx aws.Context,
	input *ecr.BatchDeleteImageInput,
	opts ...request.Option,
) (*ecr.BatchDeleteImageOutput, error) {
	if opts != nil {
		return nil, fmt.Errorf("opts must be nil")
	}
	if input.RegistryId != nil {
		return nil, fmt.Errorf("input.RegistryId must be nil")
	}
	if input.RepositoryName == nil {
		return nil, fmt.Errorf("input.RepositoryName must not be nil")
	}
	deletedImageIDs := make([]*ecr.ImageIdentifier, 0, len(input.ImageIds))
	for _, imageID := range input.ImageIds {
		if imageID.ImageTag == nil || imageID.ImageDigest != nil {
			return nil, fmt.Errorf("input.ImageIds must contain only non-nil ImageTag fields")
		}
		deletedImageIDs = append(deletedImageIDs, imageID)
		m.deletedCount++
	}
	return &ecr.BatchDeleteImageOutput{
		Failures: []*ecr.ImageFailure{},
		ImageIds: deletedImageIDs,
	}, nil
}

func (m mockedClient) DeletedCount() int {
	return m.deletedCount
}

func TestGarbageCollector_PruneAllRepos(t *testing.T) {
	until := time.Now().UTC()
	tests := []struct {
		Name                         string
		Repositories                 []*ecr.Repository
		TagsByResourceARN            map[string][]*ecr.Tag
		ImageDetailsByRepositoryName map[string][]*ecr.ImageDetail
		Opts                         []Option
		Until                        time.Time
		Excluded                     []string
		Pruned                       []string
		DeletedCount                 int
	}{
		{
			Name: "",
			Repositories: []*ecr.Repository{
				{
					RepositoryArn: aws.String(
						"arn:aws:ecr:us-east-1:000123456789:repository/thermite",
					),
					RepositoryName: aws.String("thermite"),
					RepositoryUri: aws.String(
						"000123456789.dkr.ecr.us-east-1.amazonaws.com/thermite",
					),
				},
				{
					RepositoryArn: aws.String(
						"arn:aws:ecr:us-east-1:000123456789:repository/golang",
					),
					RepositoryName: aws.String("golang"),
					RepositoryUri: aws.String(
						"000123456789.dkr.ecr.us-east-1.amazonaws.com/golang",
					),
				},
				{
					RepositoryArn: aws.String(
						"arn:aws:ecr:us-east-1:000123456789:repository/amazonlinux",
					),
					RepositoryName: aws.String("amazonlinux"),
					RepositoryUri: aws.String(
						"000123456789.dkr.ecr.us-east-1.amazonaws.com/amazonlinux",
					),
				},
			},
			TagsByResourceARN: map[string][]*ecr.Tag{
				"arn:aws:ecr:us-east-1:000123456789:repository/thermite": {
					{
						Key:   aws.String("thermite:prune-period"),
						Value: aws.String("30"),
					},
				},
				"arn:aws:ecr:us-east-1:000123456789:repository/golang": {
					{
						Key:   aws.String("thermite:prune-period"),
						Value: aws.String("0"),
					},
				},
				"arn:aws:ecr:us-east-1:000123456789:repository/amazonlinux": {},
			},
			ImageDetailsByRepositoryName: map[string][]*ecr.ImageDetail{
				"thermite": {
					{
						ImagePushedAt: aws.Time(until.Add(-(30*24 + 2) * time.Hour)),
						ImageTags: []*string{
							aws.String("0437aec133abca7f3d054a5be48dde8ed9b2af22"),
						},
					},
					{
						ImagePushedAt: aws.Time(until.Add(-(30*24 + 1) * time.Hour)),
						ImageTags: []*string{
							aws.String("878d0cb2b7e6f6017c096fa613b1b521b95325a6"),
						},
					},
					{
						ImagePushedAt: aws.Time(until.Add(-(30*24 - 1) * time.Hour)),
						ImageTags: []*string{
							aws.String("5379a3dcddb42eb007a68ea7990c643066263fb8"),
						},
					},
				},
				"golang": {
					{
						ImagePushedAt: aws.Time(time.Time{}),
						ImageTags: []*string{
							aws.String("1.15"),
						},
					},
				},
				"amazonlinux": {
					{
						ImagePushedAt: aws.Time(time.Time{}),
						ImageTags: []*string{
							aws.String("2.0.20201218.1"),
						},
					},
				},
			},
			Opts:  []Option{WithRemoveImages()},
			Until: until,
			Excluded: []string{
				"000123456789.dkr.ecr.us-east-1.amazonaws.com/thermite:0437aec133abca7f3d054a5be48dde8ed9b2af22",
			},
			Pruned: []string{
				"000123456789.dkr.ecr.us-east-1.amazonaws.com/thermite:878d0cb2b7e6f6017c096fa613b1b521b95325a6",
			},
			DeletedCount: 1,
		},
		{
			Name: "ExcludeMostRecent",
			Repositories: []*ecr.Repository{
				{
					RepositoryArn: aws.String(
						"arn:aws:ecr:us-east-1:000123456789:repository/thermite",
					),
					RepositoryName: aws.String("thermite"),
					RepositoryUri: aws.String(
						"000123456789.dkr.ecr.us-east-1.amazonaws.com/thermite",
					),
				},
			},
			TagsByResourceARN: map[string][]*ecr.Tag{
				"arn:aws:ecr:us-east-1:000123456789:repository/thermite": {
					{
						Key:   aws.String("thermite:prune-period"),
						Value: aws.String("30"),
					},
				},
			},
			ImageDetailsByRepositoryName: map[string][]*ecr.ImageDetail{
				"thermite": {
					{
						ImagePushedAt: aws.Time(until.Add(-(30*24 + 1) * time.Hour)),
						ImageTags: []*string{
							aws.String("0437aec133abca7f3d054a5be48dde8ed9b2af22"),
						},
					},
					{
						ImagePushedAt: aws.Time(until.Add(-(30*24 + 2) * time.Hour)),
						ImageTags: []*string{
							aws.String("878d0cb2b7e6f6017c096fa613b1b521b95325a6"),
						},
					},
				},
			},
			Opts:  []Option{WithRemoveImages()},
			Until: until,
			Excluded: []string{
				"000123456789.dkr.ecr.us-east-1.amazonaws.com/thermite:5379a3dcddb42eb007a68ea7990c643066263fb8",
			},
			Pruned: []string{
				"000123456789.dkr.ecr.us-east-1.amazonaws.com/thermite:878d0cb2b7e6f6017c096fa613b1b521b95325a6",
			},
			DeletedCount: 1,
		},
		{
			Name: "WithDryRun",
			Repositories: []*ecr.Repository{
				{
					RepositoryArn: aws.String(
						"arn:aws:ecr:us-east-1:000123456789:repository/thermite",
					),
					RepositoryName: aws.String("thermite"),
					RepositoryUri: aws.String(
						"000123456789.dkr.ecr.us-east-1.amazonaws.com/thermite",
					),
				},
				{
					RepositoryArn: aws.String(
						"arn:aws:ecr:us-east-1:000123456789:repository/golang",
					),
					RepositoryName: aws.String("golang"),
					RepositoryUri: aws.String(
						"000123456789.dkr.ecr.us-east-1.amazonaws.com/golang",
					),
				},
				{
					RepositoryArn: aws.String(
						"arn:aws:ecr:us-east-1:000123456789:repository/amazonlinux",
					),
					RepositoryName: aws.String("amazonlinux"),
					RepositoryUri: aws.String(
						"000123456789.dkr.ecr.us-east-1.amazonaws.com/amazonlinux",
					),
				},
			},
			TagsByResourceARN: map[string][]*ecr.Tag{
				"arn:aws:ecr:us-east-1:000123456789:repository/thermite": {
					{
						Key:   aws.String("thermite:prune-period"),
						Value: aws.String("30"),
					},
				},
				"arn:aws:ecr:us-east-1:000123456789:repository/golang": {
					{
						Key:   aws.String("thermite:prune-period"),
						Value: aws.String("0"),
					},
				},
				"arn:aws:ecr:us-east-1:000123456789:repository/amazonlinux": {},
			},
			ImageDetailsByRepositoryName: map[string][]*ecr.ImageDetail{
				"thermite": {
					{
						ImagePushedAt: aws.Time(until.Add(-(30*24 + 2) * time.Hour)),
						ImageTags: []*string{
							aws.String("0437aec133abca7f3d054a5be48dde8ed9b2af22"),
						},
					},
					{
						ImagePushedAt: aws.Time(until.Add(-(30*24 + 1) * time.Hour)),
						ImageTags: []*string{
							aws.String("878d0cb2b7e6f6017c096fa613b1b521b95325a6"),
						},
					},
					{
						ImagePushedAt: aws.Time(until.Add(-(30*24 - 1) * time.Hour)),
						ImageTags: []*string{
							aws.String("5379a3dcddb42eb007a68ea7990c643066263fb8"),
						},
					},
				},
				"golang": {
					{
						ImagePushedAt: aws.Time(time.Time{}),
						ImageTags: []*string{
							aws.String("1.15"),
						},
					},
				},
				"amazonlinux": {
					{
						ImagePushedAt: aws.Time(time.Time{}),
						ImageTags: []*string{
							aws.String("2.0.20201218.1"),
						},
					},
				},
			},
			Until: until,
			Excluded: []string{
				"000123456789.dkr.ecr.us-east-1.amazonaws.com/thermite:0437aec133abca7f3d054a5be48dde8ed9b2af22",
			},
			Pruned: []string{
				"000123456789.dkr.ecr.us-east-1.amazonaws.com/thermite:878d0cb2b7e6f6017c096fa613b1b521b95325a6",
			},
			DeletedCount: 0,
		},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			client := &mockedClient{
				Repositories:                 test.Repositories,
				TagsByResourceARN:            test.TagsByResourceARN,
				ImageDetailsByRepositoryName: test.ImageDetailsByRepositoryName,
			}
			gc, err := NewClient(client, test.Opts...)
			if err != nil {
				t.Fatal(err)
			}
			gotPruned, err := gc.PruneAllRepos(
				context.Background(),
				test.Until,
				test.Excluded...,
			)
			if err != nil {
				t.Fatal(err)
			}
			sort.Strings(test.Pruned)
			sort.Strings(gotPruned)
			if diff := cmp.Diff(test.Pruned, gotPruned); diff != "" {
				t.Fatal(diff)
			}
			gotDeletedCount := client.DeletedCount()
			if diff := cmp.Diff(test.DeletedCount, gotDeletedCount); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}
