package main

import (
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type prometheusFeeder struct {
	environment string
	ticker      *time.Ticker
	controller  controller
}

func newPrometheusFeeder(environment string, controller controller) *prometheusFeeder {
	ticker := time.NewTicker(60 * time.Second)
	return &prometheusFeeder{
		environment: environment,
		ticker:      ticker,
		controller:  controller,
	}
}

func (g prometheusFeeder) feed() {
	setPilotLight(g.environment)
	serviceStatus := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "upp",
			Subsystem: "health",
			Name:      "servicestatus",
			Help:      "Status of the service: 0 - healthy; 1 - unhealthy",
		},
		[]string{
			"environment",
			"service",
		})
	prometheus.MustRegister(serviceStatus)
	for range g.ticker.C {
		for _, mService := range g.controller.getMeasuredServices() {
			for {
				select {
				case checkResult := <-mService.bufferedHealths.buffer:
					name := strings.Replace(checkResult.Name, ".", "-", -1)
					checkStatus := inverseBoolToInt(checkResult.Ok)
					serviceStatus.With(prometheus.Labels{"environment": g.environment, "service": name}).Set(float64(checkStatus))
				}
			}
		}
	}
}

func setPilotLight(environment string) {
	pilotLight := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "upp",
			Subsystem: "health",
			Name:      "pilotlight",
			Help:      "Pilot light for the service monitoring UPP service health",
		},
		[]string{
			"environment",
		})
	prometheus.MustRegister(pilotLight)
	pilotLight.With(prometheus.Labels{"environment": environment}).Set(1)
}
