package main

import (
	"context"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/elbv2/elbv2iface"
	log "github.com/sirupsen/logrus"
)

func (s *scanner) scanAllELBv2(ctx context.Context) {
	err := s.forRegions(
		endpoints.ElasticloadbalancingServiceID,
		func(region string, session *session.Session) {
			elbClient := elbv2.New(session, &aws.Config{
				Region: aws.String(region),
			})
			log.WithFields(log.Fields{
				"service": "elbv2",
				"region":  region,
			}).Info("starting scan of region")
			if err := s.scanELBv2(ctx, region, elbClient); err != nil {
				log.WithFields(log.Fields{
					log.ErrorKey: err,
					"service":    "elbv2",
					"region":     region,
				}).Error("error scanning region")
				return
			}
		},
	)
	if err != nil {
		log.WithFields(log.Fields{
			log.ErrorKey: err,
			"service":    "elbv2",
		}).Error("error iterating over regions")
	}
}

func (s *scanner) scanELBv2(ctx context.Context, region string, client elbv2iface.ELBV2API) error {
	return client.DescribeLoadBalancersPagesWithContext(
		ctx,
		&elbv2.DescribeLoadBalancersInput{},
		func(out *elbv2.DescribeLoadBalancersOutput, lastPage bool) bool {
			for _, lb := range out.LoadBalancers {
				if *lb.Scheme != "internet-facing" {
					continue
				}

				if lb.VpcId != nil && flagIgnoreVPCs[*lb.VpcId] {
					log.WithFields(log.Fields{
						"service": endpoints.Ec2ServiceID,
						"region":  region,
						"lb":      *lb.LoadBalancerName,
						"vpc":     *lb.VpcId,
					}).Debug("ignoring load balancer")
					continue
				}

				// Expand via DNS
				s.resolveResource(ctx, Resource{
					Region: region,
					Type:   "load-balancer",
					IP:     *lb.DNSName,
					Name:   *lb.LoadBalancerName,
				})
			}
			return true
		},
	)
	return nil
}
