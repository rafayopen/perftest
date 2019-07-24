package cw

import (
	pt "github.com/rafayopen/perftest/pkg/pt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"

	"log"
	"time"
)

// Publish resonse time in milliseconds as metric "RespTime" in CloudWatch namespace "Perf Demo".
// Could be made more generic of course.  This is for Rafay Feb Demo A.
//
// AWS Cloudwatch requires the following variables in the environment (see AWS SDK docs):
// AWS_REGION
// AWS_ACCESS_KEY_ID
// AWS_SECRET_ACCESS_KEY
func PublishRespTime(location, url, respCode string, respTime float64) {
	// Load credentials from the shared credentials file ~/.aws/credentials
	// and configuration from the shared configuration file ~/.aws/config.
	sess := session.Must(session.NewSession())
	// If the session cannot be created this will panic the application !!

	// Create new cloudwatch client.
	svc := cloudwatch.New(sess)

	// static metric and namespace for now
	metric := "RespTime"
	namespace := "Http Perf Demo"

	timestamp := time.Now()
	_, err := svc.PutMetricData(&cloudwatch.PutMetricDataInput{
		Namespace: aws.String(namespace),

		MetricData: []*cloudwatch.MetricDatum{
			&cloudwatch.MetricDatum{
				Timestamp:  &timestamp,
				MetricName: aws.String(metric),
				Value:      aws.Float64(respTime),
				Unit:       aws.String(cloudwatch.StandardUnitMilliseconds),
				Dimensions: []*cloudwatch.Dimension{
					&cloudwatch.Dimension{
						Name:  aws.String("TestUrl"),
						Value: aws.String(url),
					},
					&cloudwatch.Dimension{
						Name:  aws.String("HTTP Resp Code"),
						Value: aws.String(respCode),
					},
					&cloudwatch.Dimension{
						Name:  aws.String("FromLocation"),
						Value: aws.String(pt.LocationOrIp(&location)),
					},
				},
			},
		},
	})
	if err != nil {
		log.Println("Error publishing", url, "from", location, "to cloudwatch:", err)
	}
}
