package resource

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/aws-cloudformation/cloudformation-cli-go-plugin/cfn/handler"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/stretchr/testify/assert"
)

func TestInitialize(t *testing.T) {
	m := &Model{
		ClusterID: aws.String("eks"),
		Chart:     aws.String("stable/coscale"),
		Namespace: aws.String("default"),
	}
	vpc := &VPCConfiguration{
		SecurityGroupIds: []string{"sg-01"},
		SubnetIds:        []string{"subnet-01"},
	}
	data := []byte("Test")
	_ = ioutil.WriteFile(KubeConfigLocalPath, data, 0644)
	_ = ioutil.WriteFile(ZipFile, data, 0644)
	defer os.Remove(KubeConfigLocalPath)
	defer os.Remove(ZipFile)
	tests := map[string]struct {
		action    Action
		vpc       bool
		name      string
		nextStage Stage
	}{
		"InstallWithVPC": {
			action:    InstallReleaseAction,
			name:      "Test",
			vpc:       true,
			nextStage: ReleaseStabilize,
		},
		"InstallWithOutVPC": {
			action:    InstallReleaseAction,
			name:      "Test",
			vpc:       false,
			nextStage: ReleaseStabilize,
		},
		"UpdateWithOutVPC": {
			action:    UpdateReleaseAction,
			name:      "one",
			vpc:       false,
			nextStage: ReleaseStabilize,
		},
		"UpdateWithVPC": {
			action:    UpdateReleaseAction,
			name:      "one",
			vpc:       true,
			nextStage: ReleaseStabilize,
		},
		"UninstallsWithOutVPC": {
			action:    UninstallReleaseAction,
			name:      "one",
			vpc:       false,
			nextStage: CompleteStage,
		},
		"UninstallWithVPC": {
			action:    UninstallReleaseAction,
			name:      "one",
			vpc:       true,
			nextStage: CompleteStage,
		},
	}

	NewClients = func(cluster *string, kubeconfig *string, namespace *string, ses *session.Session, role *string, customKubeconfig []byte) (*Clients, error) {
		return NewMockClient(t), nil
	}

	for name, d := range tests {
		t.Run(name, func(t *testing.T) {
			if d.vpc {
				m.VPCConfiguration = vpc
			}
			m.Name = aws.String(d.name)
			m.ID = nil
			eRes := makeEvent(m, d.nextStage, nil)
			res := initialize(MockSession, m, d.action)
			assert.EqualValues(t, eRes, res)
		})
	}
}

func TestCheckReleaseStatus(t *testing.T) {
	m := &Model{
		ClusterID: aws.String("eks"),
		ID:        aws.String("eyJDbHVzdGVySUQiOiJla3MiLCJSZWdpb24iOiJldS13ZXN0LTEiLCJOYW1lIjoiVGVzdCIsIk5hbWVzcGFjZSI6IlRlc3QifQ"),
	}
	vpc := &VPCConfiguration{
		SecurityGroupIds: []string{"sg-01"},
		SubnetIds:        []string{"subnet-01"},
	}
	c := NewMockClient(t)
	data := []byte("Test")
	_ = ioutil.WriteFile(KubeConfigLocalPath, data, 0644)
	_ = ioutil.WriteFile(ZipFile, data, 0644)
	defer os.Remove(KubeConfigLocalPath)
	defer os.Remove(ZipFile)
	tests := map[string]struct {
		vpc       bool
		name      *string
		nextStage Stage
	}{
		"WithVPC": {
			name:      aws.String("one"),
			vpc:       true,
			nextStage: CompleteStage,
		},
		"WithOutVPC": {
			name:      aws.String("one"),
			vpc:       false,
			nextStage: CompleteStage,
		},
	}

	NewClients = func(cluster *string, kubeconfig *string, namespace *string, ses *session.Session, role *string, customKubeconfig []byte) (*Clients, error) {
		return NewMockClient(t), nil
	}

	for name, d := range tests {
		t.Run(name, func(t *testing.T) {
			if d.vpc {
				m.VPCConfiguration = vpc
			}
			m.Name = d.name
			eRes := makeEvent(m, d.nextStage, nil)
			res := checkReleaseStatus(c.AWSClients.Session(nil, nil), m, d.nextStage)
			assert.EqualValues(t, eRes, res)
		})
	}
}
func TestLambdaDestroy(t *testing.T) {
	m := &Model{
		ClusterID: aws.String("eks"),
		VPCConfiguration: &VPCConfiguration{
			SecurityGroupIds: []string{"sg-1"},
			SubnetIds:        []string{"subnet-1"},
		},
	}
	expected := handler.ProgressEvent{OperationStatus: "SUCCESS", HandlerErrorCode: "", Message: "", CallbackContext: map[string]interface{}(nil), CallbackDelaySeconds: 0, ResourceModel: m, ResourceModels: []interface{}(nil), NextToken: ""}
	c := NewMockClient(t)
	result := c.lambdaDestroy(m)
	assert.EqualValues(t, expected, result)

}

func TestInitializeLambda(t *testing.T) {
	l := &lambdaResource{
		nameSuffix:   aws.String("suffix"),
		functionFile: TestZipFile,
		vpcConfig: &VPCConfiguration{
			SecurityGroupIds: []string{"sg-1"},
			SubnetIds:        []string{"subnet-1"},
		},
	}
	eErr := "not in desired state"
	tests := map[string]struct {
		name      *string
		assertion assert.BoolAssertionFunc
	}{
		"StateActive": {
			name:      aws.String("function1"),
			assertion: assert.True,
		},
		"StateFailed": {
			name:      aws.String("function2"),
			assertion: assert.False,
		},
		"StateNotFound": {
			name:      aws.String("Nofunct"),
			assertion: assert.False,
		},
	}
	c := NewMockClient(t)
	for name, d := range tests {
		t.Run(name, func(t *testing.T) {
			l.functionName = d.name
			result, err := c.initializeLambda(l)
			if err != nil {
				assert.Contains(t, err.Error(), eErr)
			}
			d.assertion(t, result)
		})
	}
}

func TestHelmStatusWrapper(t *testing.T) {
	c := NewMockClient(t)
	event := &Event{
		Action: CheckReleaseAction,
	}
	name := aws.String("one")
	tests := []bool{true, false}
	functionName := aws.String("function1")
	for _, d := range tests {
		testName := "WithOutVPC"
		if d {
			testName = "WithVPC"
		}
		t.Run(testName, func(t *testing.T) {
			_, err := c.helmStatusWrapper(name, event, functionName, d)
			assert.Nil(t, err)
		})
	}
}

func TestHelmListWrapper(t *testing.T) {
	c := NewMockClient(t)
	event := &Event{
		Action: CheckReleaseAction,
		Inputs: &Inputs{
			ChartDetails: &Chart{
				Chart:     aws.String("hello-0.1.0"),
				ChartName: aws.String("hello"),
			},
			Config: &Config{
				Namespace: aws.String("default"),
			},
		},
	}

	name := aws.String("one")
	tests := []bool{true, false}
	functionName := aws.String("function1")
	for _, d := range tests {
		testName := "WithOutVPC"
		if d {
			testName = "WithVPC"
		}
		t.Run(testName, func(t *testing.T) {
			_, err := c.helmListWrapper(name, event, functionName, d)
			assert.Nil(t, err)
		})
	}
}
func TestHelmInstallWrapper(t *testing.T) {
	defer os.Remove(chartLocalPath)
	testServer := httptest.NewServer(http.StripPrefix("/", http.FileServer(http.Dir(TestFolder))))
	defer func() { testServer.Close() }()
	c := NewMockClient(t)
	event := &Event{
		Action: InstallReleaseAction,
		Inputs: &Inputs{
			Config: &Config{
				Name:      aws.String("test"),
				Namespace: aws.String("default"),
			},
			ValueOpts: map[string]interface{}{},
		},
	}
	event.Inputs.ChartDetails, _ = getChartDetails(&Model{Chart: aws.String(testServer.URL + "/test.tgz")})
	tests := []bool{true, false}
	functionName := aws.String("function1")
	for _, d := range tests {
		testName := "WithOutVPC"
		if d {
			testName = "WithVPC"
		}
		t.Run(testName, func(t *testing.T) {
			err := c.helmInstallWrapper(event, functionName, d)
			assert.Nil(t, err)
		})
	}
}

func TestHelmUpgradeWrapper(t *testing.T) {
	testServer := httptest.NewServer(http.StripPrefix("/", http.FileServer(http.Dir(TestFolder))))
	defer func() { testServer.Close() }()
	c := NewMockClient(t)
	event := &Event{
		Action: UpdateReleaseAction,
		Inputs: &Inputs{
			Config: &Config{
				Name:      aws.String("test"),
				Namespace: aws.String("default"),
			},
			ValueOpts: map[string]interface{}{},
		},
	}
	event.Inputs.ChartDetails, _ = getChartDetails(&Model{Chart: aws.String(testServer.URL + "/test.tgz")})
	name := aws.String("one")
	tests := []bool{true, false}
	functionName := aws.String("function1")
	for _, d := range tests {
		testName := "WithOutVPC"
		if d {
			testName = "WithVPC"
		}
		t.Run(testName, func(t *testing.T) {
			err := c.helmUpgradeWrapper(name, event, functionName, d)
			assert.Nil(t, err)
		})
	}
}

func TestHelmDeleteWrapper(t *testing.T) {
	c := NewMockClient(t)
	event := &Event{
		Action: UninstallReleaseAction,
	}
	name := aws.String("one")
	tests := []bool{true, false}
	functionName := aws.String("function1")
	for _, d := range tests {
		testName := "WithOutVPC"
		if d {
			testName = "WithVPC"
		}
		t.Run(testName, func(t *testing.T) {
			err := c.helmDeleteWrapper(name, event, functionName, d)
			assert.Nil(t, err)
		})
	}
}

func TestKubePendingWrapper(t *testing.T) {
	c := NewMockClient(t)
	event := &Event{
		Action: GetPendingAction,
		ReleaseData: &ReleaseData{
			Name:      "one",
			Namespace: "default",
			Manifest:  TestManifest,
		},
	}
	name := aws.String("one")
	tests := []bool{true, false}
	functionName := aws.String("function1")
	for _, d := range tests {
		testName := "WithOutVPC"
		if d {
			testName = "WithVPC"
		}
		t.Run(testName, func(t *testing.T) {
			_, err := c.kubePendingWrapper(name, event, functionName, d)
			assert.Nil(t, err)
		})
	}
}

func TestKubeResourcesWrapper(t *testing.T) {
	c := NewMockClient(t)
	event := &Event{
		Action: GetResourcesAction,
		ReleaseData: &ReleaseData{
			Name:      "one",
			Namespace: "default",
			Manifest:  TestManifest,
		},
	}
	name := aws.String("one")
	tests := []bool{true, false}
	functionName := aws.String("function1")
	for _, d := range tests {
		testName := "WithOutVPC"
		if d {
			testName = "WithVPC"
		}
		t.Run(testName, func(t *testing.T) {
			_, err := c.kubeResourcesWrapper(name, event, functionName, d)
			assert.Nil(t, err)
		})
	}
}
