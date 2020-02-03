package main

import (
	"context"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/aws/aws-sdk-go/service/elb/elbiface"
	log "github.com/sirupsen/logrus"
)

func (s *scanner) scanAllELB(ctx context.Context) {
	err := s.forRegions(
		endpoints.ElasticloadbalancingServiceID,
		func(region string, session *session.Session) {
			elbClient := elb.New(session, &aws.Config{
				Region: aws.String(region),
			})
			log.WithFields(log.Fields{
				"service": endpoints.ElasticloadbalancingServiceID,
				"region":  region,
			}).Info("starting scan of region")
			if err := s.scanELB(ctx, region, elbClient); err != nil {
				log.WithFields(log.Fields{
					log.ErrorKey: err,
					"service":    endpoints.ElasticloadbalancingServiceID,
					"region":     region,
				}).Error("error scanning region")
				return
			}
		},
	)
	if err != nil {
		log.WithFields(log.Fields{
			log.ErrorKey: err,
			"service":    endpoints.ElasticloadbalancingServiceID,
		}).Error("error iterating over regions")
	}
}

func (s *scanner) scanELB(ctx context.Context, region string, client elbiface.ELBAPI) error {
	return client.DescribeLoadBalancersPagesWithContext(
		ctx,
		&elb.DescribeLoadBalancersInput{},
		func(out *elb.DescribeLoadBalancersOutput, lastPage bool) bool {
			for _, lb := range out.LoadBalancerDescriptions {
				if *lb.Scheme != "internet-facing" {
					continue
				}

				if lb.VPCId != nil && flagIgnoreVPCs[*lb.VPCId] {
					log.WithFields(log.Fields{
						"service": endpoints.Ec2ServiceID,
						"region":  region,
						"lb":      *lb.LoadBalancerName,
						"vpc":     *lb.VPCId,
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
