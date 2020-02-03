package main

import (
	"context"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/aws/aws-sdk-go/service/rds/rdsiface"
	log "github.com/sirupsen/logrus"
)

func (s *scanner) scanAllRDS(ctx context.Context) {
	err := s.forRegions(
		endpoints.ElasticloadbalancingServiceID,
		func(region string, session *session.Session) {
			rdsClient := rds.New(session, &aws.Config{
				Region: aws.String(region),
			})
			log.WithFields(log.Fields{
				"service": endpoints.ElasticloadbalancingServiceID,
				"region":  region,
			}).Info("starting scan of region")
			if err := s.scanRDS(ctx, region, rdsClient); err != nil {
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

func (s *scanner) scanRDS(ctx context.Context, region string, client rdsiface.RDSAPI) error {
	return client.DescribeDBInstancesPagesWithContext(
		ctx,
		&rds.DescribeDBInstancesInput{},
		func(out *rds.DescribeDBInstancesOutput, lastPage bool) bool {
			for _, db := range out.DBInstances {
				if !*db.PubliclyAccessible {
					continue
				}

				if db.DBSubnetGroup != nil && db.DBSubnetGroup.VpcId != nil && flagIgnoreVPCs[*db.DBSubnetGroup.VpcId] {
					log.WithFields(log.Fields{
						"service": endpoints.Ec2ServiceID,
						"region":  region,
						"db":      *db.DBName,
						"vpc":     *db.DBSubnetGroup.VpcId,
					}).Debug("ignoring load balancer")
					continue
				}

				// Expand via DNS
				s.resolveResource(ctx, Resource{
					Region: region,
					Type:   "db-instance",
					IP:     *db.Endpoint.Address,
					Name:   *db.DBName,
				})
			}
			return true
		},
	)
	return nil
}
