package main

import (
	"encoding/json"
	"errors"
	"fmt"
	fthealth "github.com/Financial-Times/go-fthealth/v1a"
	"io/ioutil"
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/labels"
	"net/http"
)

type healthcheckResponse struct {
	Name   string
	Checks []struct {
		Name     string
		OK       bool
		Severity uint8
	}
}

func (hs *k8sHealthcheckService) checkServiceHealth(serviceName string) (string, error) {
	k8sDeployments, err := hs.k8sClient.Extensions().Deployments("default").List(api.ListOptions{LabelSelector: labels.SelectorFromSet(labels.Set{"app": serviceName})})
	if err != nil {
		return "", fmt.Errorf("Error retrieving deployment with label app=%s", serviceName)
	}

	if len(k8sDeployments.Items) == 0 {
		return "", fmt.Errorf("Cannot find deployment with label app=%s", serviceName)
	}

	noOfUnavailablePods := k8sDeployments.Items[0].Status.UnavailableReplicas
	noOfAvailablePods := k8sDeployments.Items[0].Status.AvailableReplicas

	if noOfAvailablePods == 0 {
		return "", errors.New("All pods are unavailable")
	}

	if noOfUnavailablePods != 0 {
		return fmt.Sprintf("There are %v pods unavailable", noOfUnavailablePods), nil
	}

	return "", nil
}

func (hs *k8sHealthcheckService) checkPodHealth(pod pod) error {
	health, err := hs.getHealthChecksForPod(pod)
	if err != nil {
		errorLogger.Printf("Cannot perform healthcheck for pod. Error was: %s", err.Error())
		return errors.New("Cannot perform healthcheck for pod")
	}

	for _, check := range health.Checks {
		if check.OK != true {
			return fmt.Errorf("Failing check is: %s", check.Name)
		}
	}

	return nil
}

func (hs *k8sHealthcheckService) getIndividualPodSeverity(pod pod) (uint8, error) {
	health, err := hs.getHealthChecksForPod(pod)

	if err != nil {
		return defaultSeverity, fmt.Errorf("Cannot get severity for pod with name %s. Error was: %s", pod.name, err.Error())
	}

	finalSeverity := uint8(2)
	for _, check := range health.Checks {
		if check.OK != true {
			if check.Severity < finalSeverity {
				return check.Severity, nil
			}
		}
	}

	return finalSeverity, nil
}

func (hs *k8sHealthcheckService) getHealthChecksForPod(pod pod) (healthcheckResponse, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("http://%s:8080/__health", pod.ip), nil)
	if err != nil {
		return healthcheckResponse{}, errors.New("Error constructing healthcheck request: " + err.Error())
	}

	resp, err := hs.httpClient.Do(req)
	if err != nil {
		return healthcheckResponse{}, errors.New("Error performing healthcheck request: " + err.Error())
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return healthcheckResponse{}, fmt.Errorf("Healthcheck endpoint returned non-200 status (%v)", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return healthcheckResponse{}, errors.New("Error reading healthcheck response: " + err.Error())
	}

	health := &healthcheckResponse{}
	if err := json.Unmarshal(body, &health); err != nil {
		return healthcheckResponse{}, errors.New("Error parsing healthcheck response: " + err.Error())
	}

	return *health, nil
}

func newPodHealthCheck(pod pod, service service, healthcheckService healthcheckService) fthealth.Check {
	return fthealth.Check{
		BusinessImpact:   "On its own this failure does not have a business impact but it represents a degradation of the cluster health.",
		Name:             pod.name,
		PanicGuide:       "https://sites.google.com/a/ft.com/technology/systems/dynamic-semantic-publishing/coco/runbook",
		Severity:         defaultSeverity,
		TechnicalSummary: "The service is not healthy. Please check the panic guide.",
		Checker: func() (string, error) {
			return "", healthcheckService.checkPodHealth(pod)
		},
	}
}

func newServiceHealthCheck(service service, healthcheckService healthcheckService) fthealth.Check {
	return fthealth.Check{
		BusinessImpact:   "On its own this failure does not have a business impact but it represents a degradation of the cluster health.",
		Name:             service.name,
		PanicGuide:       "https://sites.google.com/a/ft.com/technology/systems/dynamic-semantic-publishing/coco/runbook",
		Severity:         defaultSeverity,
		TechnicalSummary: "The service is not healthy. Please check the panic guide.",
		Checker: func() (string, error) {
			return healthcheckService.checkServiceHealth(service.name)
		},
	}
}