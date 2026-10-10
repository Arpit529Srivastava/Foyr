// Package awsx implements the build interfaces with AWS SDK v2: CodeBuild, ECR, SSM and CloudWatch Logs.
package awsx

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/codebuild"
	cbtypes "github.com/aws/aws-sdk-go-v2/service/codebuild/types"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"

	"github.com/Arpit529Srivastava/Foyr/internal/build"
)

// keepImages is how many images each app repository keeps.
const keepImages = 10

// Builds runs the shared CodeBuild project.
type Builds struct {
	Client  *codebuild.Client
	Project string
}

// NewBuilds returns Builds for project.
func NewBuilds(cfg aws.Config, project string) *Builds {
	return &Builds{Client: codebuild.NewFromConfig(cfg), Project: project}
}

// Start starts one build; the clone token is passed by SSM name so CodeBuild masks it.
func (b *Builds) Start(ctx context.Context, r build.Request) (build.Build, error) {
	plain := func(name, value string) cbtypes.EnvironmentVariable {
		return cbtypes.EnvironmentVariable{Name: aws.String(name), Value: aws.String(value),
			Type: cbtypes.EnvironmentVariableTypePlaintext}
	}
	out, err := b.Client.StartBuild(ctx, &codebuild.StartBuildInput{
		ProjectName: aws.String(b.Project),
		EnvironmentVariablesOverride: []cbtypes.EnvironmentVariable{
			plain("REPO_URL", r.RepoURL),
			plain("COMMIT", r.Commit),
			plain("IMAGE_URI", r.ImageURI),
			plain("FOYR_DEPLOYMENT_ID", strconv.FormatInt(r.DeploymentID, 10)),
			{Name: aws.String("GITHUB_TOKEN"), Value: aws.String(r.TokenParam),
				Type: cbtypes.EnvironmentVariableTypeParameterStore},
		},
	})
	if err != nil {
		return build.Build{}, err
	}
	return convert(out.Build), nil
}

// Get returns the current state of one build.
func (b *Builds) Get(ctx context.Context, id string) (build.Build, error) {
	out, err := b.Client.BatchGetBuilds(ctx, &codebuild.BatchGetBuildsInput{Ids: []string{id}})
	if err != nil {
		return build.Build{}, err
	}
	if len(out.Builds) == 0 {
		return build.Build{}, fmt.Errorf("build %s not found", id)
	}
	return convert(&out.Builds[0]), nil
}

func convert(in *cbtypes.Build) build.Build {
	if in == nil {
		return build.Build{}
	}
	b := build.Build{ID: aws.ToString(in.Id), Status: string(in.BuildStatus)}
	if l := in.Logs; l != nil {
		b.LogURL, b.LogGroup, b.LogStream = aws.ToString(l.DeepLink), aws.ToString(l.GroupName), aws.ToString(l.StreamName)
	}
	for _, p := range in.Phases {
		ph := build.Phase{Type: string(p.PhaseType), Status: string(p.PhaseStatus), Start: p.StartTime, End: p.EndTime}
		for _, c := range p.Contexts {
			if m := aws.ToString(c.Message); m != "" {
				ph.Message = joinMsg(ph.Message, aws.ToString(c.StatusCode)+": "+m)
			}
		}
		b.Phases = append(b.Phases, ph)
	}
	return b
}

func joinMsg(a, b string) string {
	if a == "" {
		return b
	}
	return a + "; " + b
}

// Registry creates per-app ECR repositories on first use.
type Registry struct {
	Client *ecr.Client
	known  sync.Map // name -> URI
}

// NewRegistry returns a Registry.
func NewRegistry(cfg aws.Config) *Registry { return &Registry{Client: ecr.NewFromConfig(cfg)} }

// EnsureRepo returns the URI of repository name, creating it with immutable tags,
// scanning and a keep-last-10 lifecycle rule if it does not exist.
func (r *Registry) EnsureRepo(ctx context.Context, name string) (string, error) {
	if uri, ok := r.known.Load(name); ok {
		return uri.(string), nil
	}
	out, err := r.Client.DescribeRepositories(ctx, &ecr.DescribeRepositoriesInput{RepositoryNames: []string{name}})
	var notFound *ecrtypes.RepositoryNotFoundException
	switch {
	case err == nil && len(out.Repositories) > 0:
		uri := aws.ToString(out.Repositories[0].RepositoryUri)
		r.known.Store(name, uri)
		return uri, nil
	case err != nil && !errors.As(err, &notFound):
		return "", err
	}

	created, err := r.Client.CreateRepository(ctx, &ecr.CreateRepositoryInput{
		RepositoryName:             aws.String(name),
		ImageTagMutability:         ecrtypes.ImageTagMutabilityImmutable,
		ImageScanningConfiguration: &ecrtypes.ImageScanningConfiguration{ScanOnPush: true},
		Tags:                       []ecrtypes.Tag{{Key: aws.String("foyr:managed-by"), Value: aws.String("foyrd")}},
	})
	var exists *ecrtypes.RepositoryAlreadyExistsException
	if errors.As(err, &exists) { // another worker created it first
		return r.EnsureRepo(ctx, name)
	}
	if err != nil {
		return "", err
	}
	policy := fmt.Sprintf(`{"rules":[{"rulePriority":1,"description":"Keep the last %d images",`+
		`"selection":{"tagStatus":"any","countType":"imageCountMoreThan","countNumber":%d},"action":{"type":"expire"}}]}`,
		keepImages, keepImages)
	if _, err := r.Client.PutLifecyclePolicy(ctx, &ecr.PutLifecyclePolicyInput{
		RepositoryName: aws.String(name), LifecyclePolicyText: aws.String(policy)}); err != nil {
		return "", fmt.Errorf("lifecycle policy: %w", err)
	}
	uri := aws.ToString(created.Repository.RepositoryUri)
	r.known.Store(name, uri)
	return uri, nil
}

// Secrets stores build tokens in SSM Parameter Store as SecureStrings.
type Secrets struct{ Client *ssm.Client }

// NewSecrets returns Secrets.
func NewSecrets(cfg aws.Config) *Secrets { return &Secrets{Client: ssm.NewFromConfig(cfg)} }

// Put writes value encrypted, replacing any earlier value.
func (s *Secrets) Put(ctx context.Context, name, value string) error {
	_, err := s.Client.PutParameter(ctx, &ssm.PutParameterInput{
		Name: aws.String(name), Value: aws.String(value), Type: ssmtypes.ParameterTypeSecureString,
		Overwrite: aws.Bool(true), Tier: ssmtypes.ParameterTierStandard,
	})
	return err
}

// Delete removes name; a missing parameter is not an error.
func (s *Secrets) Delete(ctx context.Context, name string) error {
	_, err := s.Client.DeleteParameter(ctx, &ssm.DeleteParameterInput{Name: aws.String(name)})
	var notFound *ssmtypes.ParameterNotFound
	if errors.As(err, &notFound) {
		return nil
	}
	return err
}

// Logs reads build logs from CloudWatch.
type Logs struct{ Client *cloudwatchlogs.Client }

// NewLogs returns Logs.
func NewLogs(cfg aws.Config) *Logs { return &Logs{Client: cloudwatchlogs.NewFromConfig(cfg)} }

// Tail returns the last n lines of a log stream.
func (l *Logs) Tail(ctx context.Context, group, stream string, n int) ([]string, error) {
	if group == "" || stream == "" {
		return nil, nil
	}
	out, err := l.Client.GetLogEvents(ctx, &cloudwatchlogs.GetLogEventsInput{
		LogGroupName: aws.String(group), LogStreamName: aws.String(stream),
		Limit: aws.Int32(int32(n)), StartFromHead: aws.Bool(false),
	})
	if err != nil {
		return nil, err
	}
	lines := make([]string, 0, len(out.Events))
	for _, e := range out.Events {
		lines = append(lines, trimNewline(aws.ToString(e.Message)))
	}
	return lines, nil
}

func trimNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
