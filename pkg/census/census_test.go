package census

import (
	"context"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	batchV1beta1 "k8s.io/api/batch/v1beta1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func TestTaker_ImagesInUse(t *testing.T) {
	tests := []struct {
		Name      string
		Objects   []runtime.Object
		ImageRefs []string
	}{
		{
			Name:      "ResourceCount=0",
			Objects:   []runtime.Object{},
			ImageRefs: []string{},
		},
		{
			Name: "ResourceCount=4",
			Objects: []runtime.Object{
				&appsv1.Deployment{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Deployment",
						APIVersion: "apps/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo",
					},
					Spec: appsv1.DeploymentSpec{
						Template: v1.PodTemplateSpec{
							Spec: v1.PodSpec{
								Containers: []v1.Container{
									{
										Image: "000123456789.dkr.ecr.us-east-1.amazonaws.com/thermite:878d0cb2b7e6f6017c096fa613b1b521b95325a6",
									},
								},
							},
						},
					},
				},
				&appsv1.Deployment{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Deployment",
						APIVersion: "apps/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "bar",
					},
					Spec: appsv1.DeploymentSpec{
						Template: v1.PodTemplateSpec{
							Spec: v1.PodSpec{
								Containers: []v1.Container{
									{
										Image: "dollarshaveclub/thermite:0437aec133abca7f3d054a5be48dde8ed9b2af22",
									},
								},
							},
						},
					},
				},
				&appsv1.StatefulSet{
					TypeMeta: metav1.TypeMeta{
						Kind:       "StatefulSet",
						APIVersion: "apps/v1",
					},
					Spec: appsv1.StatefulSetSpec{
						Template: v1.PodTemplateSpec{
							Spec: v1.PodSpec{
								Containers: []v1.Container{
									{
										Image: "golang:1.15",
									},
								},
							},
						},
					},
				},
				&appsv1.DaemonSet{
					TypeMeta: metav1.TypeMeta{
						Kind:       "DaemonSet",
						APIVersion: "apps/v1",
					},
					Spec: appsv1.DaemonSetSpec{
						Template: v1.PodTemplateSpec{
							Spec: v1.PodSpec{
								Containers: []v1.Container{
									{
										Image: "000123456789.dkr.ecr.us-east-1.amazonaws.com/thermite:5379a3dcddb42eb007a68ea7990c643066263fb8",
									},
								},
							},
						},
					},
				},
				&batchV1beta1.CronJob{
					TypeMeta: metav1.TypeMeta{
						Kind:       "CronJob",
						APIVersion: "batch/v1beta1",
					},
					Spec: batchV1beta1.CronJobSpec{
						Schedule: "* * * * *",
						JobTemplate: batchV1beta1.JobTemplateSpec{
							Spec: batchv1.JobSpec{
								Template: v1.PodTemplateSpec{
									Spec: v1.PodSpec{
										Containers: []v1.Container{
											{
												Image: "000123456789.dkr.ecr.us-east-1.amazonaws.com/thermite:878d0cb2b7e6f6017c096fa613b1b521b95325a6",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			ImageRefs: []string{
				"000123456789.dkr.ecr.us-east-1.amazonaws.com/thermite:878d0cb2b7e6f6017c096fa613b1b521b95325a6",
				"dollarshaveclub/thermite:0437aec133abca7f3d054a5be48dde8ed9b2af22",
				"golang:1.15",
				"000123456789.dkr.ecr.us-east-1.amazonaws.com/thermite:5379a3dcddb42eb007a68ea7990c643066263fb8",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset(test.Objects...)
			taker, err := NewDefaultClient(clientset)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			got, err := taker.SurveyDeployedImages(ctx)
			if err != nil {
				t.Fatal(err)
			}
			sort.Strings(test.ImageRefs)
			sort.Strings(sort.StringSlice(got))
			if diff := cmp.Diff(test.ImageRefs, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}
