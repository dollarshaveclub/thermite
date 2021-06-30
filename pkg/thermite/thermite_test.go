package thermite

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

type mockedCensusClient struct {
	ImageRefs []string
}

func (m mockedCensusClient) SurveyDeployedImages(
	ctx context.Context,
) (deployed []string, err error) {
	return m.ImageRefs, nil
}

type mockedPruneClient struct {
	ImageRefsByRepo map[string][]string
}

func (m mockedPruneClient) PruneAllRepos(
	ctx context.Context,
	until time.Time,
	excluded ...string,
) (pruned []string, err error) {
	pruned = make([]string, 0, len(m.ImageRefsByRepo))
	for name := range m.ImageRefsByRepo {
		repoPruned, err := m.PruneRepo(ctx, name, until, excluded...)
		pruned = append(pruned, repoPruned...)
		if err != nil {
			return pruned, fmt.Errorf("error pruning repo %s: %w", name, err)
		}
	}
	return pruned, nil
}

func (m mockedPruneClient) PruneRepo(
	ctx context.Context,
	name string,
	until time.Time,
	excluded ...string,
) (pruned []string, err error) {
	pruned = []string{}
	imageRefs, ok := m.ImageRefsByRepo[name]
	if !ok {
		return pruned, nil
	}
	pruned = make([]string, 0, len(m.ImageRefsByRepo))
	isExcluded := make(map[string]bool, len(excluded))
	for _, imageRef := range excluded {
		isExcluded[imageRef] = true
	}
	for _, imageRef := range imageRefs {
		if isExcluded[imageRef] {
			continue
		}
		pruned = append(pruned, imageRef)
	}
	return pruned, nil
}

func TestThermite_Run(t *testing.T) {
	censusClient := mockedCensusClient{
		ImageRefs: []string{
			"thermite:0437aec133abca7f3d054a5be48dde8ed9b2af22",
			"golang:1.15",
		},
	}
	pruneClient := mockedPruneClient{
		ImageRefsByRepo: map[string][]string{
			"thermite": {
				"thermite:0437aec133abca7f3d054a5be48dde8ed9b2af22",
				"thermite:878d0cb2b7e6f6017c096fa613b1b521b95325a6",
				"thermite:5379a3dcddb42eb007a68ea7990c643066263fb8",
				"golang:1.15",
				"amazonlinux:2.0.20201218.1",
			},
		},
	}
	tests := []struct {
		Name     string
		Surveyed []string
		Old      []string
		Pruned   []string
	}{
		{
			Name: "",
			Surveyed: []string{
				"thermite:0437aec133abca7f3d054a5be48dde8ed9b2af22",
				"golang:1.15",
			},
			Old: []string{
				"thermite:0437aec133abca7f3d054a5be48dde8ed9b2af22",
				"thermite:878d0cb2b7e6f6017c096fa613b1b521b95325a6",
				"thermite:5379a3dcddb42eb007a68ea7990c643066263fb8",
				"golang:1.15",
				"amazonlinux:2.0.20201218.1",
			},
			Pruned: []string{
				"thermite:878d0cb2b7e6f6017c096fa613b1b521b95325a6",
				"thermite:5379a3dcddb42eb007a68ea7990c643066263fb8",
				"amazonlinux:2.0.20201218.1",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			client, err := NewClient(censusClient, pruneClient)
			if err != nil {
				t.Fatal(err)
			}
			got, err := client.Run(context.Background(), time.Now().UTC())
			if err != nil {
				t.Fatal(err)
			}
			sort.Strings(test.Pruned)
			sort.Strings(got)
			if diff := cmp.Diff(test.Pruned, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}

}
