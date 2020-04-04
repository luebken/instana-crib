package main

import (
	"context"
	"crypto/tls"
	"flag"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/antihax/optional"
	"github.com/nfisher/instana-crib/pkg/instana/openapi"
)

const (
	// Metric for the report
	Metric = "cpu.user"
	// Plugin to query
	Plugin = "host"
)

func newConfiguration(apiURL string, isInsecure bool) (*openapi.Configuration, error) {
	u, err := url.Parse(apiURL)
	if err != nil {
		return nil, err
	}

	httpClient := &http.Client{
		Transport: &http.Transport{
			// ignore expired SSL certificates
			TLSClientConfig: &tls.Config{InsecureSkipVerify: isInsecure},
		},
	}

	configuration := openapi.NewConfiguration()
	configuration.BasePath = apiURL
	configuration.Host = u.Hostname()
	configuration.HTTPClient = httpClient

	return configuration, nil
}

func main() {
	var apiToken = os.Getenv("INSTANA_TOKEN")
	var apiURL = os.Getenv("INSTANA_URL")
	var queryString string

	flag.StringVar(&queryString, "query", "entity.zone:us-east-2", "DFQ Query")
	flag.Parse()

	log.Printf("API Key Set: %v\n", apiToken != "")
	log.Printf("API URL:     %v\n", apiURL)

	if apiToken == "" {
		panic("INSTANA_TOKEN environment variable should be set to the Instana API token. Was a k8s secret created for this?")
	}

	if apiURL == "" {
		panic("INSTANA_URL environment variable should be set to the Instana API end-point. Was a k8s secret created for this?")
	}

	configuration, err := newConfiguration(apiURL, true)
	if err != nil {
		log.Fatal(err.Error())
	}

	client := openapi.NewAPIClient(configuration)
	ctx := context.WithValue(
		context.Background(),
		openapi.ContextAPIKey,
		openapi.APIKey{
			Key:    apiToken,
			Prefix: "apiToken",
		})

	// https://instana.github.io/openapi/#tag/Infrastructure-Metrics
	// https://docs.instana.io/core_concepts/data_collection/#data-retention
	var query = &openapi.GetInfrastructureMetricsOpts{
		GetCombinedMetrics: optional.NewInterface(openapi.GetCombinedMetrics{
			TimeFrame: openapi.TimeFrame{
				WindowSize: 3600, //in ms
				//To:  unix-timestamp
			},
			Rollup:  1, // in sec. possible values 1,5,60,300,3600
			Query:   queryString,
			Plugin:  Plugin,
			Metrics: []string{Metric},
		}),
	}

	configResp, httpResp, err := client.InfrastructureMetricsApi.GetInfrastructureMetrics(ctx, query)
	if err != nil {
		log.Fatalf("error in retrieving metrics: %s\n", err.(openapi.GenericOpenAPIError).Body())
	}

	if len(configResp.Items) < 1 {
		log.Fatalln("No metrics found")
	}
	log.Printf("Rate Limit Remaining: %#v\n", httpResp.Header.Get("X-Ratelimit-Remaining"))
	log.Println("")

	for _, item := range configResp.Items {

		log.Printf("Host: %+v\n", item.Host)
		// TODO get k8s cluster & namespace
		log.Printf("SnapshotId: %+v\n", item.SnapshotId)

		//TODO why are there multiple values ?
		millis := item.Metrics[Metric][0][0]
		ttime := time.Unix(0, int64(millis)*int64(time.Millisecond))
		value := item.Metrics[Metric][0][1]

		log.Printf("Time: %v, CPU used: %v\n", ttime, value)
		log.Println("")
	}

}
