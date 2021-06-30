// Package census surveys container image names in a Kubernetes cluster.
package census

import (
	"context"
	"fmt"
	"io"
	"log"
	"sort"

	"github.com/DataDog/datadog-go/statsd"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	batchV1beta1 "k8s.io/api/batch/v1beta1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/pager"
)

// A Taker surveys container image names in a Kubernetes cluster.
type Taker interface {
	SurveyDeployedImages(ctx context.Context) (deployed []string, err error)
}

// PodSpecLister is implemented for resource kinds which contain a PodSpec and
// can be listed via the Kubernetes API.
type PodSpecLister interface {
	// List returns the result of a List method on the clientset for the
	// resource kind associated with the PodSpecLister.
	List(ctx context.Context, clientset kubernetes.Interface) (runtime.Object, error)
	// GetPodSpec returns the PodSpec associated with obj, which will be of
	// the same type as the elements of the list returned by the List
	// method.
	GetPodSpec(ctx context.Context, obj runtime.Object) (v1.PodSpec, error)
}

// CronJobLister lists the PodSpecs of all CronJobs in a Kubernetes cluster.
var CronJobLister PodSpecLister = &cronJobLister{}

// DaemonSetLister lists the PodSpecs of all DaemonSets in a Kubernetes cluster.
var DaemonSetLister PodSpecLister = &daemonSetLister{}

// DeploymentLister lists the PodSpecs of all Deployments in a Kubernetes cluster.
var DeploymentLister PodSpecLister = &deploymentLister{}

// JobLister lists the PodSpecs of all Jobs in a Kubernetes cluster.
var JobLister PodSpecLister = &jobLister{}

// StatefulSetLister lists the PodSpecs of all StatefulSets in a Kubernetes cluster.
var StatefulSetLister PodSpecLister = &statefulSetLister{}

// A Client is a configurable Taker wrapping kubernetes.Interface.
type Client struct {
	clientset kubernetes.Interface
	listers   []PodSpecLister
	pageSize  uint
	logger    *log.Logger
	statsd    statsd.ClientInterface
}

// An Option is an option applied when creating a Client.
type Option func(c *Client)

// WithLister adds a PodSpecLister for a Client to survey.
func WithLister(lister PodSpecLister) Option {
	return func(c *Client) {
		c.listers = append(c.listers, lister)
	}
}

// WithPageSize sets the maximum number of responses a Client should request in
// a single Kubernetes API call.
func WithPageSize(size uint) Option {
	return func(c *Client) {
		if size <= 0 {
			return
		}
		c.pageSize = size
	}
}

// WithLogger sets a logger for a Client to output to.
func WithLogger(logger *log.Logger) Option {
	return func(c *Client) { c.logger = logger }
}

// WithStatsdClient sets a statsd client to use to report metrics from a Client.
func WithStatsdClient(client statsd.ClientInterface) Option {
	return func(c *Client) { c.statsd = client }
}

// NewDefaultClient returns a Taker that surveys CronJob, DaemonSet, Deployment,
// Job, and StatefulSet resources from clientset.
func NewDefaultClient(clientset kubernetes.Interface, opts ...Option) (*Client, error) {
	opts = append(
		opts,
		WithLister(DeploymentLister),
		WithLister(DaemonSetLister),
		WithLister(CronJobLister),
		WithLister(JobLister),
		WithLister(StatefulSetLister),
	)
	return NewClient(clientset, opts...)
}

// NewClient returns a Taker that surveys resources from clientset. If no
// WithLister option is included in opts, no resources will be surveyed.
func NewClient(clientset kubernetes.Interface, opts ...Option) (*Client, error) {
	if clientset == nil {
		return nil, fmt.Errorf("clientset must not be nil")
	}
	c := &Client{
		clientset: clientset,
		logger:    log.New(io.Discard, "", 0),
		statsd:    &statsd.NoOpClient{},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// SurveyDeployedImages returns the image references of the containers and init containers
// of the PodSpecs surveyed by t.
func (c *Client) SurveyDeployedImages(ctx context.Context) ([]string, error) {
	var span tracer.Span
	span, ctx = tracer.StartSpanFromContext(ctx, "census.Client.SurveyDeployedImages")
	defer span.Finish()
	defer c.statsd.Flush()
	imageSet := make(map[string]interface{})
	for _, l := range c.listers {
		pager := pager.New(func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
			return l.List(ctx, c.clientset)
		})
		if err := pager.EachListItem(
			ctx,
			metav1.ListOptions{
				Limit: int64(c.pageSize),
			},
			func(obj runtime.Object) error {
				spec, err := l.GetPodSpec(ctx, obj)
				if err != nil {
					return fmt.Errorf("error getting PodSpec from resource: %w", err)
				}
				containers := append(spec.Containers, spec.InitContainers...)
				for _, c := range containers {
					imageSet[c.Image] = nil
				}
				return nil
			},
		); err != nil {
			span.Finish(tracer.WithError(err))
			return nil, fmt.Errorf("error listing resources: %w", err)
		}
		c.logger.Printf("listed images from PodSpecLister %T", l)
	}
	imageRefs := make([]string, 0, len(imageSet))
	for image := range imageSet {
		imageRefs = append(imageRefs, image)
	}
	sort.Sort(sort.StringSlice(imageRefs))
	c.logger.Printf("surveyed %d unique deployed images", len(imageRefs))
	c.statsd.Gauge("census.survey_deployed_images", float64(len(imageRefs)), nil, 1)
	return imageRefs, nil
}

type cronJobLister struct{}

func (l *cronJobLister) List(ctx context.Context, clientset kubernetes.Interface) (runtime.Object, error) {
	list, err := clientset.BatchV1beta1().CronJobs("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error listing CronJobs: %w", err)
	}
	return list, nil
}

func (l *cronJobLister) GetPodSpec(ctx context.Context, obj runtime.Object) (v1.PodSpec, error) {
	if obj == nil {
		return v1.PodSpec{}, fmt.Errorf("obj must not be nil")
	}
	cronJob, ok := obj.(*batchV1beta1.CronJob)
	if !ok {
		return v1.PodSpec{}, fmt.Errorf(
			"error asserting type of list item as CronJob: got type %T",
			cronJob,
		)
	}
	return cronJob.Spec.JobTemplate.Spec.Template.Spec, nil
}

type daemonSetLister struct{}

func (l *daemonSetLister) List(ctx context.Context, clientset kubernetes.Interface) (runtime.Object, error) {
	list, err := clientset.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error listing DaemonSets: %w", err)
	}
	return list, nil
}

func (l *daemonSetLister) GetPodSpec(ctx context.Context, obj runtime.Object) (v1.PodSpec, error) {
	if obj == nil {
		return v1.PodSpec{}, fmt.Errorf("obj must not be nil")
	}
	daemonSet, ok := obj.(*appsv1.DaemonSet)
	if !ok {
		return v1.PodSpec{}, fmt.Errorf(
			"error asserting type of list item as DaemonSet: got type %T",
			daemonSet,
		)
	}
	return daemonSet.Spec.Template.Spec, nil
}

type deploymentLister struct{}

func (l *deploymentLister) List(ctx context.Context, clientset kubernetes.Interface) (runtime.Object, error) {
	list, err := clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error listing Deployments: %w", err)
	}
	return list, nil
}

func (l *deploymentLister) GetPodSpec(ctx context.Context, obj runtime.Object) (v1.PodSpec, error) {
	if obj == nil {
		return v1.PodSpec{}, fmt.Errorf("obj must not be nil")
	}
	deployment, ok := obj.(*appsv1.Deployment)
	if !ok {
		return v1.PodSpec{}, fmt.Errorf(
			"error asserting type of list item as Deployment: got type %T",
			deployment,
		)
	}
	return deployment.Spec.Template.Spec, nil
}

type jobLister struct{}

func (l *jobLister) List(ctx context.Context, clientset kubernetes.Interface) (runtime.Object, error) {
	list, err := clientset.BatchV1().Jobs("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error listing Jobs: %w", err)
	}
	return list, nil
}

func (l *jobLister) GetPodSpec(ctx context.Context, obj runtime.Object) (v1.PodSpec, error) {
	if obj == nil {
		return v1.PodSpec{}, fmt.Errorf("obj must not be nil")
	}
	job, ok := obj.(*batchv1.Job)
	if !ok {
		return v1.PodSpec{}, fmt.Errorf(
			"error asserting type of list item as Job: got type %T",
			job,
		)
	}
	return job.Spec.Template.Spec, nil
}

type statefulSetLister struct{}

func (l *statefulSetLister) List(ctx context.Context, clientset kubernetes.Interface) (runtime.Object, error) {
	list, err := clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error listing DaemonSets: %w", err)
	}
	return list, nil
}

func (l *statefulSetLister) GetPodSpec(ctx context.Context, obj runtime.Object) (v1.PodSpec, error) {
	if obj == nil {
		return v1.PodSpec{}, fmt.Errorf("obj must not be nil")
	}
	statefulSet, ok := obj.(*appsv1.StatefulSet)
	if !ok {
		return v1.PodSpec{}, fmt.Errorf(
			"error asserting type of list item as StatefulSet: got type %T",
			statefulSet,
		)
	}
	return statefulSet.Spec.Template.Spec, nil
}
