package main

import (
	"github.com/Financial-Times/go-logger"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	v1beta1app "k8s.io/api/apps/v1beta1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func init() {
	logger.InitLogger("upp-aggregate-healthcheck", "debug")
}

type mockTransport struct {
	responseStatusCode int
	responseBody       string
}

const (
	validIP                             = "1.0.0.0"
	validK8sServiceName                 = "validServiceName"
	validK8sServiceNameWithAck          = "validK8sServiceNameWithAck"
	validSeverity                       = uint8(1)
	ackMsg                              = "ack-msg"
	validFailingHealthCheckResponseBody = `{
  "schemaVersion": 1,
  "name": "CMSNotifierApplication",
  "description": "CMSNotifierApplication",
  "checks": [
    {
      "checkOutput": "",
      "panicGuide": "",
      "lastUpdated": "",
      "ok": false,
      "severity": 2,
      "businessImpact": "",
      "technicalSummary": "",
      "name": ""
    },
	{
      "checkOutput": "",
      "panicGuide": "",
      "lastUpdated": "",
      "ok": false,
      "severity": 1,
      "businessImpact": "",
      "technicalSummary": "",
      "name": ""
    }
  ]
}`
	validFailingHealthCheckResponseBodyWithSeverity2 = `{
  "schemaVersion": 1,
  "name": "CMSNotifierApplication",
  "description": "CMSNotifierApplication",
  "checks": [
    {
      "checkOutput": "",
      "panicGuide": "",
      "lastUpdated": "",
      "ok": false,
      "severity": 2,
      "businessImpact": "",
      "technicalSummary": "",
      "name": ""
    }
  ]
}`
	validPassingHealthCheckResponseBody = `{
  "schemaVersion": 1,
  "name": "CMSNotifierApplication",
  "description": "CMSNotifierApplication",
  "checks": [
    {
      "checkOutput": "",
      "panicGuide": "",
      "lastUpdated": "",
      "ok": true,
      "severity": 2,
      "businessImpact": "",
      "technicalSummary": "",
      "name": ""
    }
  ]
}`
)

func initializeMockServiceWithK8sServices() *k8sHealthcheckService {
	services := make(map[string]service)
	services[validK8sServiceName] = service{
		name: validServiceName,
	}
	services[validK8sServiceNameWithAck] = service{
		name: validK8sServiceNameWithAck,
		ack:  "test",
	}
	return &k8sHealthcheckService{
		services: servicesMap{
			m: services,
		},
	}
}

func initializeMockService(httpClient *http.Client) *k8sHealthcheckService {
	mockK8sClient := fake.NewSimpleClientset()

	return &k8sHealthcheckService{
		k8sClient:  mockK8sClient,
		httpClient: httpClient,
	}
}

func initializeMockHTTPClient(responseStatusCode int, responseBody string) *http.Client {
	client := getDefaultClient()
	client.Transport = &mockTransport{
		responseStatusCode: responseStatusCode,
		responseBody:       responseBody,
	}

	return client
}

func (t *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	response := &http.Response{
		Header:     make(http.Header),
		Request:    req,
		StatusCode: t.responseStatusCode,
	}

	response.Header.Set("Content-Type", "application/json")
	response.Body = ioutil.NopCloser(strings.NewReader(t.responseBody))

	return response, nil
}

func TestGetHealthChecksForPodInternalServerErr(t *testing.T) {
	service := initializeMockService(initializeMockHTTPClient(http.StatusInternalServerError, ""))
	_, err := service.getHealthChecksForPod(pod{name: "test", ip: validIP}, 8080)
	assert.NotNil(t, err)
}

func TestGetHealthChecksForPodHealthAvailable(t *testing.T) {
	service := initializeMockService(initializeMockHTTPClient(http.StatusOK, validFailingHealthCheckResponseBody))
	healthCheckResponse, err := service.getHealthChecksForPod(pod{name: "test", ip: validIP}, 8080)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(healthCheckResponse.Checks))
}

func TestGetIndividualPodSeverityErrorWhilePodHealthCheck(t *testing.T) {
	service := initializeMockService(initializeMockHTTPClient(http.StatusInternalServerError, ""))
	severity, _, err := service.getIndividualPodSeverity(pod{name: "test", ip: validIP}, 8080)
	assert.NotNil(t, err)
	assert.Equal(t, defaultSeverity, severity)
}

func TestGetIndividualPodSeverityValidPodHealth(t *testing.T) {
	service := initializeMockService(initializeMockHTTPClient(http.StatusOK, validFailingHealthCheckResponseBody))
	severity, checkFailed, err := service.getIndividualPodSeverity(pod{name: "test", ip: validIP}, 8080)
	assert.Nil(t, err)
	assert.True(t, checkFailed)
	assert.Equal(t, validSeverity, severity)
}

func TestGetIndividualPodSeverityValidPodHealth_Severity2(t *testing.T) {
	service := initializeMockService(initializeMockHTTPClient(http.StatusOK, validFailingHealthCheckResponseBodyWithSeverity2))
	severity, checkFailed, err := service.getIndividualPodSeverity(pod{name: "test", ip: validIP}, 8080)
	assert.Nil(t, err)
	assert.True(t, checkFailed)
	assert.Equal(t, uint8(2), severity)
}

func TestCheckPodHealthFailingChecks(t *testing.T) {
	service := initializeMockService(initializeMockHTTPClient(http.StatusOK, validFailingHealthCheckResponseBody))
	err := service.checkPodHealth(pod{name: "test", ip: validIP}, 8080)
	assert.NotNil(t, err)
}

func TestCheckPodHealthWithInvalidUrl(t *testing.T) {
	service := initializeMockService(nil)
	err := service.checkPodHealth(pod{name: "test", ip: "%s"}, 8080)
	assert.NotNil(t, err)
}

func TestCheckPodHealthPassingChecks(t *testing.T) {
	service := initializeMockService(initializeMockHTTPClient(http.StatusOK, validPassingHealthCheckResponseBody))
	err := service.checkPodHealth(pod{name: "test", ip: validIP}, 8080)
	assert.Nil(t, err)
}

func TestGetCategories(t *testing.T) {
	service := initializeMockService(nil)
	_, err := service.getCategories()
	assert.Nil(t, err)
}

func TestUpdateCategoryInvalidConfigMap(t *testing.T) {
	service := initializeMockService(nil)
	err := service.updateCategory("validCategoryName", true)
	assert.NotNil(t, err)
}

func TestAddAckConfigMapNotFound(t *testing.T) {
	service := initializeMockService(nil)
	err := service.addAck("invalidServiceName", "ack message")
	assert.NotNil(t, err)
}

func TestUpdateAcksForServicesEmptyAckList(t *testing.T) {
	hcService := initializeMockServiceWithK8sServices()
	acks := make(map[string]string)
	acks[validK8sServiceName] = ackMsg
	hcService.updateAcksForServices(acks)

	assert.Equal(t, hcService.services.m[validK8sServiceNameWithAck].ack, "")
	assert.Equal(t, hcService.services.m[validK8sServiceName].ack, ackMsg)
}

func TestGetDeploymentsReturnsDeployments(t *testing.T) {
	service := initializeMockService(nil)
	var replicas int32 = 1
	_, err := service.k8sClient.ExtensionsV1beta1().Deployments(namespace).Create(
		&v1beta1.Deployment{
			ObjectMeta: v1.ObjectMeta{
				Name:      "deployment1",
				Namespace: namespace,
			},
			Spec: v1beta1.DeploymentSpec{
				Replicas: &replicas,
			},
		})
	assert.Nil(t, err)

	_, err = service.k8sClient.ExtensionsV1beta1().Deployments(namespace).Create(
		&v1beta1.Deployment{
			ObjectMeta: v1.ObjectMeta{
				Name:      "deployment2",
				Namespace: namespace,
			},
			Spec: v1beta1.DeploymentSpec{
				Replicas: &replicas,
			},
		})
	assert.Nil(t, err)

	deployments, err := service.getDeployments()

	assert.Nil(t, err)
	assert.Equal(t, 2, len(deployments))
	assertDeploymentsHas(t, deployments, "deployment1")
	assertDeploymentsHas(t, deployments, "deployment2")
}

func TestGetDeploymentsReturnsDeploymentsAndStatefulSets(t *testing.T) {
	service := initializeMockService(nil)
	var replicas int32 = 1
	_, err := service.k8sClient.ExtensionsV1beta1().Deployments(namespace).Create(
		&v1beta1.Deployment{
			ObjectMeta: v1.ObjectMeta{
				Name:      "deployment1",
				Namespace: namespace,
			},
			Spec: v1beta1.DeploymentSpec{
				Replicas: &replicas,
			},
		})
	assert.Nil(t, err)

	_, err = service.k8sClient.ExtensionsV1beta1().Deployments(namespace).Create(
		&v1beta1.Deployment{
			ObjectMeta: v1.ObjectMeta{
				Name:      "deployment2",
				Namespace: namespace,
			},
			Spec: v1beta1.DeploymentSpec{
				Replicas: &replicas,
			},
		})
	assert.Nil(t, err)

	_, err = service.k8sClient.AppsV1beta1().StatefulSets(namespace).Create(
		&v1beta1app.StatefulSet{
			ObjectMeta: v1.ObjectMeta{
				Name:      "deployment3",
				Namespace: namespace,
			},
			Spec: v1beta1app.StatefulSetSpec{
				ServiceName: "special-stateful-service",
				Replicas:    &replicas,
			},
		})
	assert.Nil(t, err)

	deployments, err := service.getDeployments()

	assert.Nil(t, err)
	assert.Equal(t, 3, len(deployments))
	assertDeploymentsHas(t, deployments, "deployment1")
	assertDeploymentsHas(t, deployments, "deployment2")
	assertDeploymentsHas(t, deployments, "special-stateful-service")
}

func assertDeploymentsHas(t *testing.T, deployments map[string]deployment, key string) {
	_, present := deployments[key]
	assert.True(t, present, "Expected deployments to have %s", key)
}

func TestGetDefaultClient(t *testing.T) {
	hc := getDefaultClient()
	assert.Equal(t, hc.Timeout, 12*time.Second, "Expected time out to be 12 seconds")
}
