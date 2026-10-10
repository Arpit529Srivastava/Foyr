package awsx

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"

	"github.com/Arpit529Srivastava/Foyr/internal/build"
)

var ctx = context.Background()

type call struct {
	op   string
	body map[string]any
}

// fakeAWS answers JSON-protocol calls by operation name (the part after the dot in X-Amz-Target).
type fakeAWS struct {
	mu    sync.Mutex
	calls []call
	reply map[string]func(body map[string]any) (int, string)
}

func (f *fakeAWS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	target := r.Header.Get("X-Amz-Target")
	op := target[strings.LastIndex(target, ".")+1:]
	f.mu.Lock()
	f.calls = append(f.calls, call{op, body})
	h := f.reply[op]
	f.mu.Unlock()
	status, out := http.StatusOK, "{}"
	if h != nil {
		status, out = h(body)
	}
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, out)
}

func (f *fakeAWS) ops() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, c := range f.calls {
		out = append(out, c.op)
	}
	return out
}

func newFake(t *testing.T, reply map[string]func(map[string]any) (int, string)) (*fakeAWS, aws.Config) {
	t.Helper()
	f := &fakeAWS{reply: reply}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return f, aws.Config{
		Region:       "ap-south-1",
		Credentials:  credentials.NewStaticCredentialsProvider("AKIDTEST", "secret", ""),
		BaseEndpoint: aws.String(srv.URL),
		Retryer:      func() aws.Retryer { return aws.NopRetryer{} },
	}
}

func awsErr(typ string) (int, string) {
	return http.StatusBadRequest, `{"__type":"` + typ + `","message":"nope"}`
}

func TestStartBuildPassesTokenByParameterName(t *testing.T) {
	f, cfg := newFake(t, map[string]func(map[string]any) (int, string){
		"StartBuild": func(map[string]any) (int, string) {
			return 200, `{"build":{"id":"foyr-build:1","buildStatus":"IN_PROGRESS",
				"logs":{"deepLink":"https://console/log","groupName":"/foyr/builds","streamName":"s1"}}}`
		},
	})
	b, err := NewBuilds(cfg, "foyr-build").Start(ctx, build.Request{RepoURL: "https://github.com/o/r.git",
		Commit: strings.Repeat("a", 40), ImageURI: "repo:tag", TokenParam: "/foyr/builds/d-1/token", DeploymentID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if b.ID != "foyr-build:1" || b.LogURL != "https://console/log" || b.LogGroup != "/foyr/builds" || b.LogStream != "s1" {
		t.Fatalf("build = %+v", b)
	}
	body := f.calls[0].body
	if body["projectName"] != "foyr-build" {
		t.Fatalf("project = %v", body["projectName"])
	}
	env := map[string]map[string]any{}
	for _, v := range body["environmentVariablesOverride"].([]any) {
		m := v.(map[string]any)
		env[m["name"].(string)] = m
	}
	if tok := env["GITHUB_TOKEN"]; tok["type"] != "PARAMETER_STORE" || tok["value"] != "/foyr/builds/d-1/token" {
		t.Fatalf("GITHUB_TOKEN env = %v", tok)
	}
	for name, want := range map[string]string{"REPO_URL": "https://github.com/o/r.git", "IMAGE_URI": "repo:tag",
		"COMMIT": strings.Repeat("a", 40), "FOYR_DEPLOYMENT_ID": "1"} {
		if env[name]["value"] != want || env[name]["type"] != "PLAINTEXT" {
			t.Errorf("%s = %v", name, env[name])
		}
	}
}

func TestGetBuildConvertsPhases(t *testing.T) {
	_, cfg := newFake(t, map[string]func(map[string]any) (int, string){
		"BatchGetBuilds": func(map[string]any) (int, string) {
			return 200, `{"builds":[{"id":"foyr-build:1","buildStatus":"FAILED","phases":[
				{"phaseType":"PRE_BUILD","phaseStatus":"SUCCEEDED","startTime":1760000000,"endTime":1760000003},
				{"phaseType":"BUILD","phaseStatus":"FAILED","startTime":1760000003,"endTime":1760000060,
				 "contexts":[{"statusCode":"COMMAND_EXECUTION_ERROR","message":"exit status 1"}]}]}]}`
		},
	})
	b, err := NewBuilds(cfg, "foyr-build").Get(ctx, "foyr-build:1")
	if err != nil {
		t.Fatal(err)
	}
	if b.Status != "FAILED" || len(b.Phases) != 2 || b.Phases[1].Message != "COMMAND_EXECUTION_ERROR: exit status 1" ||
		b.Phases[1].End.Sub(*b.Phases[1].Start).Seconds() != 57 {
		t.Fatalf("build = %+v", b)
	}
}

func TestGetMissingBuild(t *testing.T) {
	_, cfg := newFake(t, map[string]func(map[string]any) (int, string){
		"BatchGetBuilds": func(map[string]any) (int, string) { return 200, `{"builds":[]}` },
	})
	if _, err := NewBuilds(cfg, "p").Get(ctx, "x"); err == nil {
		t.Fatal("want error for a missing build")
	}
}

func TestEnsureRepoCreatesOnceWithPolicy(t *testing.T) {
	f, cfg := newFake(t, map[string]func(map[string]any) (int, string){
		"DescribeRepositories": func(map[string]any) (int, string) { return awsErr("RepositoryNotFoundException") },
		"CreateRepository": func(map[string]any) (int, string) {
			return 200, `{"repository":{"repositoryUri":"123.dkr.ecr.ap-south-1.amazonaws.com/apps/1"}}`
		},
	})
	reg := NewRegistry(cfg)
	for range 2 {
		uri, err := reg.EnsureRepo(ctx, "apps/1")
		if err != nil || uri != "123.dkr.ecr.ap-south-1.amazonaws.com/apps/1" {
			t.Fatalf("uri=%q err=%v", uri, err)
		}
	}
	if got := strings.Join(f.ops(), ","); got != "DescribeRepositories,CreateRepository,PutLifecyclePolicy" {
		t.Fatalf("calls = %s (second call must use the cache)", got)
	}
	create := f.calls[1].body
	if create["imageTagMutability"] != "IMMUTABLE" ||
		create["imageScanningConfiguration"].(map[string]any)["scanOnPush"] != true {
		t.Fatalf("create = %v", create)
	}
	if p := f.calls[2].body["lifecyclePolicyText"].(string); !strings.Contains(p, `"countNumber":10`) {
		t.Fatalf("policy = %s", p)
	}
}

func TestEnsureRepoExisting(t *testing.T) {
	f, cfg := newFake(t, map[string]func(map[string]any) (int, string){
		"DescribeRepositories": func(map[string]any) (int, string) {
			return 200, `{"repositories":[{"repositoryUri":"uri/apps/2"}]}`
		},
	})
	uri, err := NewRegistry(cfg).EnsureRepo(ctx, "apps/2")
	if err != nil || uri != "uri/apps/2" || len(f.ops()) != 1 {
		t.Fatalf("uri=%q err=%v calls=%v", uri, err, f.ops())
	}
}

func TestEnsureRepoOtherError(t *testing.T) {
	_, cfg := newFake(t, map[string]func(map[string]any) (int, string){
		"DescribeRepositories": func(map[string]any) (int, string) { return awsErr("AccessDeniedException") },
	})
	if _, err := NewRegistry(cfg).EnsureRepo(ctx, "apps/3"); err == nil {
		t.Fatal("want access error")
	}
}

func TestSecretsPutAndDelete(t *testing.T) {
	f, cfg := newFake(t, map[string]func(map[string]any) (int, string){
		"DeleteParameter": func(map[string]any) (int, string) { return awsErr("ParameterNotFound") },
	})
	s := NewSecrets(cfg)
	if err := s.Put(ctx, "/foyr/builds/d-1/token", "ghs_x"); err != nil {
		t.Fatal(err)
	}
	put := f.calls[0].body
	if put["Type"] != "SecureString" || put["Overwrite"] != true || put["Name"] != "/foyr/builds/d-1/token" {
		t.Fatalf("put = %v", put)
	}
	if err := s.Delete(ctx, "/foyr/builds/d-1/token"); err != nil {
		t.Fatalf("deleting a missing parameter must succeed: %v", err)
	}
}

func TestLogsTail(t *testing.T) {
	f, cfg := newFake(t, map[string]func(map[string]any) (int, string){
		"GetLogEvents": func(map[string]any) (int, string) {
			return 200, `{"events":[{"message":"step 1\n"},{"message":"npm ERR! boom\r\n"}]}`
		},
	})
	l := NewLogs(cfg)
	lines, err := l.Tail(ctx, "/foyr/builds", "s1", 30)
	if err != nil || strings.Join(lines, "|") != "step 1|npm ERR! boom" {
		t.Fatalf("lines = %q err=%v", lines, err)
	}
	req := f.calls[0].body
	if req["limit"] != float64(30) || req["startFromHead"] != false {
		t.Fatalf("request = %v", req)
	}
	if lines, err := l.Tail(ctx, "", "", 30); err != nil || lines != nil || len(f.ops()) != 1 {
		t.Fatal("empty log location must not call AWS")
	}
}
