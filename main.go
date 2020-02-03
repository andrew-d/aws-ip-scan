package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	log "github.com/sirupsen/logrus"
)

var (
	flagIgnoreVPCs StringSet = make(StringSet)
	flagDebug                = flag.Bool("debug", false, "show debug logs")
	flagQuiet                = flag.Bool("quiet", false, "be more quiet")
)

func init() {
	flag.Var(&flagIgnoreVPCs, "ignore-vpc", "ignore resources in the given VPC(s)")
}

type Resource struct {
	Region string `json:"region"`
	Type   string `json:"type"`
	IP     string `json:"ip"`
	Name   string `json:"name"`
}

type scanner struct {
	wg             sync.WaitGroup
	resources      chan Resource
	sessions       sync.Map
	enabledRegions map[string]bool
	resolver       *net.Resolver
}

func (s *scanner) scan(ctx context.Context) error {
	session, err := s.sessionForRegion("us-east-1")
	if err != nil {
		return err
	}
	ec2Client := ec2.New(session, &aws.Config{
		Region: aws.String("us-east-1"),
	})

	// First, get all regions for EC2 to avoid running on areas where it's
	// not enabled.
	regions, err := ec2Client.DescribeRegionsWithContext(ctx, &ec2.DescribeRegionsInput{})
	if err != nil {
		return fmt.Errorf("DescribeRegionsWithContext(): %w", err)
	}

	s.enabledRegions = make(map[string]bool)
	for _, region := range regions.Regions {
		s.enabledRegions[*region.RegionName] = true
	}

	s.scanAllEC2(ctx)
	s.scanAllELB(ctx)
	s.scanAllELBv2(ctx)
	s.scanAllRDS(ctx)
	return nil
}

func (s *scanner) sessionForRegion(region string) (*session.Session, error) {
	if sess, ok := s.sessions.Load(region); ok {
		return sess.(*session.Session), nil
	}

	awsSession, err := session.NewSession(&aws.Config{
		Region: aws.String(region),
	})
	if err != nil {
		return nil, fmt.Errorf("session.NewSession(%q): %w", region, err)
	}
	s.sessions.Store(region, awsSession)
	return awsSession, nil
}

func (s *scanner) forRegions(svc string, f func(string, *session.Session)) error {
	regions, exists := endpoints.RegionsForService(
		endpoints.DefaultPartitions(),
		endpoints.AwsPartitionID,
		svc,
	)
	if !exists {
		return fmt.Errorf("Error getting regions for service: %s", svc)
	}

	// Now, scan every region in parallel.
	for _, region := range regions {
		region := region.ID()
		if !s.enabledRegions[region] {
			log.WithFields(log.Fields{
				"service": svc,
				"region":  region,
			}).Warn("skipping non-enabled region")
			continue
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			session, err := s.sessionForRegion(region)
			if err != nil {
				log.WithFields(log.Fields{
					log.ErrorKey: err,
					"service":    svc,
					"region":     region,
				}).Error("error getting session")
				return
			}

			f(region, session)
		}()
	}

	return nil
}

func main() {
	log.SetOutput(os.Stderr)
	flag.Parse()

	if *flagQuiet {
		log.SetLevel(log.WarnLevel)
	}
	if *flagDebug {
		log.SetLevel(log.DebugLevel)
	}

	s := &scanner{
		resources: make(chan Resource),
		resolver:  &net.Resolver{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Top-level waitgroup
	var wg sync.WaitGroup

	// IP printer
	wg.Add(1)
	go func() {
		defer wg.Done()

		enc := json.NewEncoder(os.Stdout)
		for {
			select {
			case <-ctx.Done():
				return
			case res := <-s.resources:
				if err := enc.Encode(&res); err != nil {
					log.WithFields(log.Fields{
						log.ErrorKey: err,
					}).Error("error encoding resource")
				}
			}
		}
	}()

	// Run scanner, wait for it to finish
	if err := s.scan(ctx); err != nil {
		log.Print(err)
	}
	s.wg.Wait()

	// Cancel our context to signal everything else to finish, then wait.
	cancel()
	wg.Wait()

	log.Println("finished")
}
