package awsx

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	sd "github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	sdtypes "github.com/aws/aws-sdk-go-v2/service/servicediscovery/types"

	"github.com/Arpit529Srivastava/Foyr/internal/run"
)

// Runtime runs deployments as ECS Fargate services registered in Cloud Map (ADR-029).
type Runtime struct {
	ECS           *ecs.Client
	SD            *sd.Client
	Cluster       string
	Region        string
	Subnets       []string
	SecurityGroup string
	ExecRoleARN   string
	LogGroup      string
	NamespaceID   string
	Capacity      string // FARGATE or FARGATE_SPOT
	CPU, Memory   string // e.g. "256", "512"
}

// NewRuntime returns a Runtime with clients from cfg; fill in the network and role fields.
func NewRuntime(cfg aws.Config) *Runtime {
	return &Runtime{ECS: ecs.NewFromConfig(cfg), SD: sd.NewFromConfig(cfg), Region: cfg.Region,
		Capacity: "FARGATE_SPOT", CPU: "256", Memory: "512"}
}

// RegisterTask registers one task definition revision: one essential container, no task role.
func (r *Runtime) RegisterTask(ctx context.Context, s run.TaskSpec) (string, error) {
	env := make([]ecstypes.KeyValuePair, 0, len(s.Env))
	for k, v := range s.Env {
		env = append(env, ecstypes.KeyValuePair{Name: aws.String(k), Value: aws.String(v)})
	}
	out, err := r.ECS.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family:                  aws.String(s.Family),
		RequiresCompatibilities: []ecstypes.Compatibility{ecstypes.CompatibilityFargate},
		NetworkMode:             ecstypes.NetworkModeAwsvpc,
		Cpu:                     aws.String(r.CPU),
		Memory:                  aws.String(r.Memory),
		ExecutionRoleArn:        aws.String(r.ExecRoleARN),
		RuntimePlatform: &ecstypes.RuntimePlatform{CpuArchitecture: ecstypes.CPUArchitectureX8664,
			OperatingSystemFamily: ecstypes.OSFamilyLinux},
		ContainerDefinitions: []ecstypes.ContainerDefinition{{
			Name:         aws.String("app"),
			Image:        aws.String(s.Image),
			Essential:    aws.Bool(true),
			PortMappings: []ecstypes.PortMapping{{ContainerPort: aws.Int32(run.AppPort), Protocol: ecstypes.TransportProtocolTcp}},
			Environment:  env,
			LogConfiguration: &ecstypes.LogConfiguration{LogDriver: ecstypes.LogDriverAwslogs, Options: map[string]string{
				"awslogs-group": r.LogGroup, "awslogs-region": r.Region, "awslogs-stream-prefix": s.LogPrefix}},
			HealthCheck: &ecstypes.HealthCheck{
				Command:     []string{"CMD-SHELL", run.HealthCheckCommand()},
				Interval:    aws.Int32(10),
				Timeout:     aws.Int32(5),
				Retries:     aws.Int32(3),
				StartPeriod: aws.Int32(30),
			},
		}},
		Tags: []ecstypes.Tag{{Key: aws.String("foyr:managed-by"), Value: aws.String("foyrd")}},
	})
	if err != nil {
		return "", err
	}
	return aws.ToString(out.TaskDefinition.TaskDefinitionArn), nil
}

// StartService registers the Cloud Map name and creates the ECS service; both calls are idempotent by name.
func (r *Runtime) StartService(ctx context.Context, s run.ServiceSpec) error {
	registry, err := r.cloudMapService(ctx, s.Name)
	if err != nil {
		return fmt.Errorf("cloud map: %w", err)
	}
	tags := []ecstypes.Tag{
		{Key: aws.String("foyr:app"), Value: aws.String(strconv.FormatInt(s.AppID, 10))},
		{Key: aws.String("foyr:deployment"), Value: aws.String(strconv.FormatInt(s.DeploymentID, 10))},
	}
	_, err = r.ECS.CreateService(ctx, &ecs.CreateServiceInput{
		Cluster:                  aws.String(r.Cluster),
		ServiceName:              aws.String(s.Name),
		TaskDefinition:           aws.String(s.TaskDefinition),
		DesiredCount:             aws.Int32(1),
		ClientToken:              aws.String(s.Name),
		CapacityProviderStrategy: []ecstypes.CapacityProviderStrategyItem{{CapacityProvider: aws.String(r.Capacity), Weight: 1}},
		NetworkConfiguration: &ecstypes.NetworkConfiguration{AwsvpcConfiguration: &ecstypes.AwsVpcConfiguration{
			Subnets: r.Subnets, SecurityGroups: []string{r.SecurityGroup}, AssignPublicIp: ecstypes.AssignPublicIpDisabled}},
		ServiceRegistries:    []ecstypes.ServiceRegistry{{RegistryArn: aws.String(registry)}},
		EnableECSManagedTags: true,
		PropagateTags:        ecstypes.PropagateTagsService,
		Tags:                 tags,
	})
	return err
}

// cloudMapService creates (or finds) the d-<id> service in the apps namespace and returns its ARN.
func (r *Runtime) cloudMapService(ctx context.Context, name string) (string, error) {
	out, err := r.SD.CreateService(ctx, &sd.CreateServiceInput{
		Name:             aws.String(name),
		NamespaceId:      aws.String(r.NamespaceID),
		CreatorRequestId: aws.String(name),
		DnsConfig: &sdtypes.DnsConfig{RoutingPolicy: sdtypes.RoutingPolicyMultivalue,
			DnsRecords: []sdtypes.DnsRecord{{Type: sdtypes.RecordTypeA, TTL: aws.Int64(10)}}},
		HealthCheckCustomConfig: &sdtypes.HealthCheckCustomConfig{},
	})
	var exists *sdtypes.ServiceAlreadyExists
	if errors.As(err, &exists) {
		got, gerr := r.SD.GetService(ctx, &sd.GetServiceInput{Id: exists.ServiceId})
		if gerr != nil {
			return "", gerr
		}
		return aws.ToString(got.Service.Arn), nil
	}
	if err != nil {
		return "", err
	}
	return aws.ToString(out.Service.Arn), nil
}

// Tasks returns the service's running and recently stopped tasks.
func (r *Runtime) Tasks(ctx context.Context, service string) ([]run.Task, error) {
	var arns []string
	for _, status := range []ecstypes.DesiredStatus{ecstypes.DesiredStatusRunning, ecstypes.DesiredStatusStopped} {
		out, err := r.ECS.ListTasks(ctx, &ecs.ListTasksInput{Cluster: aws.String(r.Cluster),
			ServiceName: aws.String(service), DesiredStatus: status})
		if err != nil {
			return nil, err
		}
		arns = append(arns, out.TaskArns...)
	}
	if len(arns) == 0 {
		return nil, nil
	}
	out, err := r.ECS.DescribeTasks(ctx, &ecs.DescribeTasksInput{Cluster: aws.String(r.Cluster), Tasks: arns})
	if err != nil {
		return nil, err
	}
	tasks := make([]run.Task, 0, len(out.Tasks))
	for _, t := range out.Tasks {
		task := run.Task{
			ID:            taskID(aws.ToString(t.TaskArn)),
			Status:        aws.ToString(t.LastStatus),
			Health:        string(t.HealthStatus),
			StoppedReason: aws.ToString(t.StoppedReason),
			StartedAt:     t.StartedAt,
		}
		for _, c := range t.Containers {
			if aws.ToString(c.Name) == "app" {
				task.ExitCode = c.ExitCode
				if reason := aws.ToString(c.Reason); reason != "" && task.StoppedReason != "" {
					task.StoppedReason += ": " + reason
				}
			}
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

// StopService deletes the ECS service and its Cloud Map name. Already-gone pieces are fine;
// Cloud Map refuses while the task is still deregistering, and the error lets the job retry.
func (r *Runtime) StopService(ctx context.Context, service string) error {
	_, err := r.ECS.DeleteService(ctx, &ecs.DeleteServiceInput{Cluster: aws.String(r.Cluster),
		Service: aws.String(service), Force: aws.Bool(true)})
	var notFound *ecstypes.ServiceNotFoundException
	var notActive *ecstypes.ServiceNotActiveException
	if err != nil && !errors.As(err, &notFound) && !errors.As(err, &notActive) {
		return err
	}
	id, err := r.cloudMapID(ctx, service)
	if err != nil || id == "" {
		return err
	}
	_, err = r.SD.DeleteService(ctx, &sd.DeleteServiceInput{Id: aws.String(id)})
	var sdNotFound *sdtypes.ServiceNotFound
	if errors.As(err, &sdNotFound) {
		return nil
	}
	return err
}

// cloudMapID finds a service by name in the apps namespace, or "".
func (r *Runtime) cloudMapID(ctx context.Context, name string) (string, error) {
	p := sd.NewListServicesPaginator(r.SD, &sd.ListServicesInput{Filters: []sdtypes.ServiceFilter{{
		Name: sdtypes.ServiceFilterNameNamespaceId, Values: []string{r.NamespaceID}, Condition: sdtypes.FilterConditionEq}}})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return "", err
		}
		for _, s := range page.Services {
			if aws.ToString(s.Name) == name {
				return aws.ToString(s.Id), nil
			}
		}
	}
	return "", nil
}

func taskID(arn string) string { return arn[strings.LastIndex(arn, "/")+1:] }
