package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/dollarshaveclub/thermite/pkg/census"
	"github.com/dollarshaveclub/thermite/pkg/prune"
	"github.com/dollarshaveclub/thermite/pkg/thermite"
	"github.com/spf13/cobra"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
	"gopkg.in/DataDog/dd-trace-go.v1/profiler"
	"k8s.io/client-go/kubernetes"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecr"

	_ "k8s.io/client-go/plugin/pkg/client/auth/azure"
	_ "k8s.io/client-go/plugin/pkg/client/auth/exec"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	_ "k8s.io/client-go/plugin/pkg/client/auth/openstack"

	"k8s.io/client-go/tools/clientcmd"
)

var (
	removeImages    bool
	periodTagKey    string
	pageSize        uint
	statsdNamespace string
	statsdTags      []string
)

func run(logger *log.Logger) (pruned []string, err error) {
	if os.Getenv("DD_AGENT_HOST") != "" && os.Getenv("DD_TRACE_AGENT_PORT") != "" {
		tracer.Start()
		defer tracer.Stop()
		logger.Printf("started Datadog tracer")
		if err := profiler.Start(); err != nil {
			return nil, fmt.Errorf("error starting Datadog profiler: %w", err)
		}
		logger.Printf("started Datadog profiler")
		defer profiler.Stop()
	}
	span, ctx := tracer.StartSpanFromContext(
		context.Background(),
		"cmd.RootCmd.run",
	)
	defer span.Finish()
	censusOpts := []census.Option{
		census.WithLogger(logger),
	}
	pruneOpts := []prune.Option{
		prune.WithPeriodTagKey(periodTagKey),
		prune.WithLogger(logger),
	}
	if pageSize > 0 {
		censusOpts = append(censusOpts, census.WithPageSize(pageSize))
		pruneOpts = append(pruneOpts, prune.WithPageSize(pageSize))
	}
	if removeImages {
		pruneOpts = append(pruneOpts, prune.WithRemoveImages())
	}
	if os.Getenv("DD_AGENT_HOST") != "" && os.Getenv("DD_DOGSTATSD_PORT") != "" {
		client, err := statsd.New(
			"",
			statsd.WithNamespace(statsdNamespace),
			statsd.WithTags(statsdTags),
		)
		if err != nil {
			span.Finish(tracer.WithError(err))
			return nil, fmt.Errorf("error creating statsd client: %w", err)
		}
		defer client.Close()
		logger.Printf("created statsd client")
		censusOpts = append(censusOpts, census.WithStatsdClient(client))
		pruneOpts = append(pruneOpts, prune.WithStatsdClient(client))
	}
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, nil)
	config, err := kubeConfig.ClientConfig()
	if err != nil {
		span.Finish(tracer.WithError(err))
		return nil, fmt.Errorf("error creating Kubernetes config: %v", err)
	}
	logger.Printf("created Kubernetes config")
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		span.Finish(tracer.WithError(err))
		return nil, fmt.Errorf("error creating Kubernetes clientset: %v", err)
	}
	censusClient, err := census.NewDefaultClient(clientset, censusOpts...)
	if err != nil {
		span.Finish(tracer.WithError(err))
		return nil, fmt.Errorf("error crearing census client: %w", err)
	}
	logger.Printf("created census client")
	sess, err := session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	})
	if err != nil {
		span.Finish(tracer.WithError(err))
		return nil, fmt.Errorf("error creating AWS session: %w", err)
	}
	logger.Printf("created ECR session")
	ecrClient := ecr.New(sess)
	pruneClient, err := prune.NewClient(ecrClient, pruneOpts...)
	if err != nil {
		span.Finish(tracer.WithError(err))
		return nil, fmt.Errorf("error creating prune client: %w", err)
	}
	logger.Printf("created prune client")
	client, err := thermite.NewClient(censusClient, pruneClient)
	if err != nil {
		span.Finish(tracer.WithError(err))
		return nil, fmt.Errorf("error crearting Thermite client: %w", err)
	}
	log.Printf("created Thermite client")
	pruned, err = client.Run(ctx, time.Now().UTC())
	if err != nil {
		span.Finish(tracer.WithError(err))
		return nil, err
	}
	return pruned, nil
}

var RootCmd = &cobra.Command{
	Version: "0.0.18",
	Use:     "thermite",
	Short:   "Remove old and undeployed Amazon Elastic Container Registry images",
	Long: `Thermite removes old Amazon Elastic Container Registry images that are not
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
the DD_AGENT_HOST and DD_TRACE_AGENT_PORT environment variables if they are set.`,
	Run: func(cmd *cobra.Command, args []string) {
		logger := log.Default()
		pruned, err := run(logger)
		for _, imageRef := range pruned {
			fmt.Println(imageRef)
		}
		if err != nil {
			logger.Fatalf("error running Thermite: %v", err)
		}
	},
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	flags := RootCmd.Flags()
	flags.BoolVarP(
		&removeImages,
		"remove-images",
		"y",
		false,
		"enables removal of eligible images from ECR",
	)
	flags.StringVar(
		&periodTagKey,
		"period-tag-key",
		prune.DefaultPeriodTagKey,
		"AWS resource tag to check for prune period",
	)
	flags.UintVar(&pageSize, "page-size", 0, "number of items returned in paginated API responses")
	flags.StringVar(
		&statsdNamespace,
		"statsd-namespace",
		"thermite",
		"namespace to add to statsd metrics",
	)
	flags.StringSliceVar(
		&statsdTags,
		"statsd-tag",
		[]string{},
		"tag to add to statsd metrics (supports multiple flags)",
	)
}
