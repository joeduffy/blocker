package main

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
)

func (d *ebsVolumeDriver) getAvailableVolumeForService(serviceName string) (*ec2.Volume, error) {
	result, err := d.ec2.DescribeVolumes(&ec2.DescribeVolumesInput{
		Filters: []*ec2.Filter{
			&ec2.Filter{
				Name:   aws.String("status"),
				Values: []*string{aws.String("available")},
			},
			&ec2.Filter{
				Name:   aws.String("availability-zone"),
				Values: []*string{aws.String(d.awsAvailabilityZone)},
			},
			&ec2.Filter{
				Name:   aws.String("tag-key"),
				Values: []*string{aws.String("service")},
			},
			&ec2.Filter{
				Name:   aws.String("tag-value"),
				Values: []*string{aws.String(serviceName)},
			},
		},
	})

	if len(result.Volumes) == 0 {
		return nil, fmt.Errorf("no volume available for service %s", serviceName)
	}

	// just return the first available one
	return result.Volumes[0], err
}
