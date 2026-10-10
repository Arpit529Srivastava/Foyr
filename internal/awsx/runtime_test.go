package awsx

import (
	"strings"
	"testing"

	"github.com/Arpit529Srivastava/Foyr/internal/run"
)

func runtime(t *testing.T, reply map[string]func(map[string]any) (int, string)) (*fakeAWS, *Runtime) {
	t.Helper()
	f, cfg := newFake(t, reply)
	r := NewRuntime(cfg)
	r.Cluster, r.Subnets, r.SecurityGroup = "foyr", []string{"subnet-a", "subnet-b"}, "sg-tenant"
	r.ExecRoleARN, r.LogGroup, r.NamespaceID = "arn:aws:iam::1:role/foyr-exec-tenant", "/foyr/apps", "ns-1"
	return f, r
}

func TestRegisterTask(t *testing.T) {
	f, r := runtime(t, map[string]func(map[string]any) (int, string){
		"RegisterTaskDefinition": func(map[string]any) (int, string) {
			return 200, `{"taskDefinition":{"taskDefinitionArn":"arn:aws:ecs:ap-south-1:1:task-definition/app-1:4"}}`
		},
	})
	arn, err := r.RegisterTask(ctx, run.TaskSpec{Family: "app-1", Image: "repo/apps/1:d2", LogPrefix: "app-1",
		Env: map[string]string{"PORT": "3000"}})
	if err != nil || arn != "arn:aws:ecs:ap-south-1:1:task-definition/app-1:4" {
		t.Fatalf("arn=%q err=%v", arn, err)
	}
	b := f.calls[0].body
	if b["family"] != "app-1" || b["networkMode"] != "awsvpc" || b["cpu"] != "256" || b["memory"] != "512" ||
		b["executionRoleArn"] != "arn:aws:iam::1:role/foyr-exec-tenant" || b["taskRoleArn"] != nil {
		t.Fatalf("task definition = %v", b)
	}
	if p := b["runtimePlatform"].(map[string]any); p["cpuArchitecture"] != "X86_64" {
		t.Fatalf("platform = %v (images are built for amd64, ADR-020)", p)
	}
	c := b["containerDefinitions"].([]any)[0].(map[string]any)
	if c["image"] != "repo/apps/1:d2" || c["essential"] != true {
		t.Fatalf("container = %v", c)
	}
	logs := c["logConfiguration"].(map[string]any)["options"].(map[string]any)
	if logs["awslogs-group"] != "/foyr/apps" || logs["awslogs-stream-prefix"] != "app-1" {
		t.Fatalf("logs = %v", logs)
	}
	hc := c["healthCheck"].(map[string]any)["command"].([]any)
	if hc[0] != "CMD-SHELL" || !strings.Contains(hc[1].(string), "/api/health") {
		t.Fatalf("health check = %v", hc)
	}
}

func TestStartServiceIsPrivateAndRegistered(t *testing.T) {
	f, r := runtime(t, map[string]func(map[string]any) (int, string){
		"CreateService": func(body map[string]any) (int, string) {
			if body["NamespaceId"] != nil { // Cloud Map
				return 200, `{"Service":{"Arn":"arn:aws:servicediscovery:ap-south-1:1:service/srv-1"}}`
			}
			return 200, `{"service":{"serviceName":"d-2"}}`
		},
	})
	if err := r.StartService(ctx, run.ServiceSpec{Name: "d-2", TaskDefinition: "arn:td", AppID: 1, DeploymentID: 2}); err != nil {
		t.Fatal(err)
	}
	cm, svc := f.calls[0].body, f.calls[1].body
	if cm["Name"] != "d-2" || cm["NamespaceId"] != "ns-1" || cm["CreatorRequestId"] != "d-2" {
		t.Fatalf("cloud map = %v", cm)
	}
	if svc["serviceName"] != "d-2" || svc["clientToken"] != "d-2" || svc["desiredCount"] != float64(1) {
		t.Fatalf("service = %v", svc)
	}
	vpc := svc["networkConfiguration"].(map[string]any)["awsvpcConfiguration"].(map[string]any)
	if vpc["assignPublicIp"] != "DISABLED" || vpc["securityGroups"].([]any)[0] != "sg-tenant" || len(vpc["subnets"].([]any)) != 2 {
		t.Fatalf("network = %v (apps must stay private in tenant subnets)", vpc)
	}
	if cp := svc["capacityProviderStrategy"].([]any)[0].(map[string]any); cp["capacityProvider"] != "FARGATE_SPOT" {
		t.Fatalf("capacity = %v", cp)
	}
	if reg := svc["serviceRegistries"].([]any)[0].(map[string]any); reg["registryArn"] != "arn:aws:servicediscovery:ap-south-1:1:service/srv-1" {
		t.Fatalf("registry = %v", reg)
	}
}

func TestStartServiceReusesCloudMapName(t *testing.T) {
	f, r := runtime(t, map[string]func(map[string]any) (int, string){
		"CreateService": func(body map[string]any) (int, string) {
			if body["NamespaceId"] != nil {
				return 400, `{"__type":"ServiceAlreadyExists","Message":"exists","ServiceId":"srv-9"}`
			}
			return 200, `{}`
		},
		"GetService": func(map[string]any) (int, string) {
			return 200, `{"Service":{"Arn":"arn:aws:servicediscovery:ap-south-1:1:service/srv-9"}}`
		},
	})
	if err := r.StartService(ctx, run.ServiceSpec{Name: "d-2", TaskDefinition: "arn:td"}); err != nil {
		t.Fatal(err)
	}
	if reg := f.calls[2].body["serviceRegistries"].([]any)[0].(map[string]any); !strings.HasSuffix(reg["registryArn"].(string), "srv-9") {
		t.Fatalf("registry = %v", reg)
	}
}

func TestTasks(t *testing.T) {
	_, r := runtime(t, map[string]func(map[string]any) (int, string){
		"ListTasks": func(body map[string]any) (int, string) {
			if body["desiredStatus"] == "STOPPED" {
				return 200, `{"taskArns":["arn:aws:ecs:ap-south-1:1:task/foyr/old"]}`
			}
			return 200, `{"taskArns":["arn:aws:ecs:ap-south-1:1:task/foyr/abc"]}`
		},
		"DescribeTasks": func(map[string]any) (int, string) {
			return 200, `{"tasks":[
				{"taskArn":"arn:aws:ecs:ap-south-1:1:task/foyr/abc","lastStatus":"RUNNING","healthStatus":"HEALTHY","startedAt":1760000000},
				{"taskArn":"arn:aws:ecs:ap-south-1:1:task/foyr/old","lastStatus":"STOPPED","stoppedReason":"Essential container in task exited",
				 "containers":[{"name":"app","exitCode":137,"reason":"OutOfMemoryError: Container killed due to memory usage"}]}]}`
		},
	})
	tasks, err := r.Tasks(ctx, "d-2")
	if err != nil || len(tasks) != 2 {
		t.Fatalf("tasks=%+v err=%v", tasks, err)
	}
	if tasks[0].ID != "abc" || tasks[0].Health != "HEALTHY" || tasks[0].StartedAt == nil {
		t.Fatalf("running = %+v", tasks[0])
	}
	if s := tasks[1]; s.Status != "STOPPED" || *s.ExitCode != 137 || !strings.Contains(s.StoppedReason, "OutOfMemoryError") {
		t.Fatalf("stopped = %+v", s)
	}
}

func TestTasksNone(t *testing.T) {
	f, r := runtime(t, map[string]func(map[string]any) (int, string){
		"ListTasks": func(map[string]any) (int, string) { return 200, `{"taskArns":[]}` },
	})
	if tasks, err := r.Tasks(ctx, "d-2"); err != nil || tasks != nil || strings.Contains(strings.Join(f.ops(), ","), "DescribeTasks") {
		t.Fatalf("tasks=%v err=%v ops=%v", tasks, err, f.ops())
	}
}

func TestStopService(t *testing.T) {
	f, r := runtime(t, map[string]func(map[string]any) (int, string){
		"DeleteService": func(body map[string]any) (int, string) {
			if body["Id"] != nil {
				return 200, `{}`
			}
			return 400, `{"__type":"ServiceNotFoundException","message":"gone"}`
		},
		"ListServices": func(map[string]any) (int, string) {
			return 200, `{"Services":[{"Id":"srv-1","Name":"d-1"},{"Id":"srv-2","Name":"d-2"}]}`
		},
	})
	if err := r.StopService(ctx, "d-2"); err != nil {
		t.Fatalf("an already deleted ECS service must not be an error: %v", err)
	}
	if got := f.calls[0].body; got["force"] != true || got["service"] != "d-2" {
		t.Fatalf("delete = %v", got)
	}
	if got := f.calls[2].body; got["Id"] != "srv-2" {
		t.Fatalf("cloud map delete = %v, want srv-2", got)
	}
}

func TestStopServiceCloudMapBusyIsRetried(t *testing.T) {
	_, r := runtime(t, map[string]func(map[string]any) (int, string){
		"DeleteService": func(body map[string]any) (int, string) {
			if body["Id"] != nil {
				return 400, `{"__type":"ResourceInUse","Message":"instances registered"}`
			}
			return 200, `{}`
		},
		"ListServices": func(map[string]any) (int, string) { return 200, `{"Services":[{"Id":"srv-2","Name":"d-2"}]}` },
	})
	if err := r.StopService(ctx, "d-2"); err == nil {
		t.Fatal("want error so the retire job retries")
	}
}
