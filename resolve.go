package main

import (
	"context"

	log "github.com/sirupsen/logrus"
)

func (s *scanner) resolveResource(ctx context.Context, resource Resource) {
	ips, err := s.resolver.LookupIPAddr(ctx, resource.IP)
	if err != nil {
		log.WithFields(log.Fields{
			log.ErrorKey: err,
			"region":     resource.Region,
			"address":    resource.IP,
		}).Error("error resolving address")
		return
	}

	for _, ip := range ips {
		s.resources <- Resource{
			Region: resource.Region,
			Type:   resource.Type,
			IP:     ip.String(),
			Name:   resource.Name,
		}
	}
}
