package main

import (
	"context"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	log "github.com/sirupsen/logrus"
)

func (s *scanner) scanAllEC2(ctx context.Context) error {
	err := s.forRegions(
		endpoints.Ec2ServiceID,
		func(region string, session *session.Session) {
			ec2Client := ec2.New(session, &aws.Config{
				Region: aws.String(region),
			})
			log.WithFields(log.Fields{
				"service": endpoints.Ec2ServiceID,
				"region":  region,
			}).Info("starting scan of region")
			if err := s.scanEC2Instances(ctx, region, ec2Client); err != nil {
				log.WithFields(log.Fields{
					log.ErrorKey: err,
					"service":    endpoints.Ec2ServiceID,
					"region":     region,
				}).Error("error scanning instances")
				return
			}
			if err := s.scanNATGateways(ctx, region, ec2Client); err != nil {
				log.WithFields(log.Fields{
					log.ErrorKey: err,
					"service":    endpoints.Ec2ServiceID,
					"region":     region,
				}).Error("error scanning NAT gateways")
				return
			}
			if err := s.scanNetworkInterfaces(ctx, region, ec2Client); err != nil {
				log.WithFields(log.Fields{
					log.ErrorKey: err,
					"service":    endpoints.Ec2ServiceID,
					"region":     region,
				}).Error("error scanning instances")
				return
			}
		},
	)
	if err != nil {
		log.WithFields(log.Fields{
			log.ErrorKey: err,
			"service":    endpoints.Ec2ServiceID,
		}).Error("error iterating over regions")
	}
	return nil
}

func (s *scanner) scanNATGateways(ctx context.Context, region string, client ec2iface.EC2API) error {
	return client.DescribeNatGatewaysPagesWithContext(
		ctx,
		&ec2.DescribeNatGatewaysInput{},
		func(out *ec2.DescribeNatGatewaysOutput, lastPage bool) bool {
			for _, gw := range out.NatGateways {
				if gw.VpcId != nil && flagIgnoreVPCs[*gw.VpcId] {
					log.WithFields(log.Fields{
						"service": endpoints.Ec2ServiceID,
						"region":  region,
						"gateway": *gw.NatGatewayId,
						"vpc":     *gw.VpcId,
					}).Debug("ignoring NAT gateway")
					continue
				}

				for _, addr := range gw.NatGatewayAddresses {
					if addr.PublicIp != nil {
						s.resources <- Resource{
							Region: region,
							Type:   "nat-gateway",
							IP:     *addr.PublicIp,
							Name:   *gw.NatGatewayId,
						}
					}
				}
			}
			return true
		},
	)
}

func (s *scanner) scanNetworkInterfaces(ctx context.Context, region string, client ec2iface.EC2API) error {
	return client.DescribeNetworkInterfacesPagesWithContext(
		ctx,
		&ec2.DescribeNetworkInterfacesInput{},
		func(out *ec2.DescribeNetworkInterfacesOutput, lastPage bool) bool {
			for _, iface := range out.NetworkInterfaces {
				if iface.VpcId != nil && flagIgnoreVPCs[*iface.VpcId] {
					log.WithFields(log.Fields{
						"service":   endpoints.Ec2ServiceID,
						"region":    region,
						"interface": *iface.NetworkInterfaceId,
						"vpc":       *iface.VpcId,
					}).Debug("ignoring network interface")
					continue
				}

				if iface.Association != nil && iface.Association.PublicIp != nil {
					s.resources <- Resource{
						Region: region,
						Type:   "network-interface",
						IP:     *iface.Association.PublicIp,
						Name:   *iface.NetworkInterfaceId,
					}
				}
			}
			return true
		},
	)
}

func (s *scanner) scanEC2Instances(ctx context.Context, region string, client ec2iface.EC2API) error {
	return client.DescribeInstancesPagesWithContext(
		ctx,
		&ec2.DescribeInstancesInput{},
		func(out *ec2.DescribeInstancesOutput, lastPage bool) bool {
			for _, reservation := range out.Reservations {
				for _, instance := range reservation.Instances {
					if instance.VpcId != nil && flagIgnoreVPCs[*instance.VpcId] {
						log.WithFields(log.Fields{
							"service":  endpoints.Ec2ServiceID,
							"region":   region,
							"instance": *instance.InstanceId,
							"vpc":      *instance.VpcId,
						}).Debug("ignoring instance")
						continue
					}

					name := getInstanceName(instance)

					// Instance public IP
					if instance.PublicIpAddress != nil {
						s.resources <- Resource{
							Region: region,
							Type:   "instance-public-ip",
							IP:     *instance.PublicIpAddress,
							Name:   name,
						}
					}

					// Public IPs on network interfaces
					for _, interf := range instance.NetworkInterfaces {
						for _, ipaddr := range interf.PrivateIpAddresses {
							if ipaddr.Association != nil && ipaddr.Association.PublicIp != nil {
								s.resources <- Resource{
									Region: region,
									Type:   "instance-network-interface",
									IP:     *ipaddr.Association.PublicIp,
									Name:   name,
								}
							}
						}
					}
				}
			}
			return true
		},
	)
}

func getInstanceName(i *ec2.Instance) string {
	for _, tag := range i.Tags {
		if tag.Key != nil && tag.Value != nil && *tag.Key == "Name" {
			return *tag.Value
		}
	}

	if i.PrivateDnsName != nil {
		return *i.PrivateDnsName
	}

	return *i.InstanceId
}
