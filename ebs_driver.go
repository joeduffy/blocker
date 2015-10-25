package main

import (
	"errors"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/service/ec2"
)

type ebsVolumeDriver struct {
	ec2                 *ec2.EC2
	ec2meta             *ec2metadata.Client
	awsInstanceID       string
	awsRegion           string
	awsAvailabilityZone string
	volumes             map[string]string
}

func NewEbsVolumeDriver() (VolumeDriver, error) {
	ec2meta := ec2metadata.New(nil)

	if !ec2meta.Available() {
		return nil, errors.New("Not running on an EC2 instance.")
	}

	d := &ebsVolumeDriver{}

	var err error
	d.awsInstanceID, err = ec2meta.GetMetadata("instance-id")
	if err != nil {
		return nil, err
	}

	d.awsRegion, err = ec2meta.Region()
	if err != nil {
		return nil, err
	}

	d.awsAvailabilityZone, err = ec2meta.GetMetadata("placement/availability-zone")
	if err != nil {
		return nil, err
	}

	return &ebsVolumeDriver{
		ec2:     ec2.New(aws.NewConfig().WithRegion(d.awsRegion)),
		ec2meta: ec2meta,
		volumes: make(map[string]string),
	}, nil
}

func (d *ebsVolumeDriver) getEbsInfo(name string) error {
	// Query EC2 to make sure this volume is indeed available.
	volumes, err := d.ec2.DescribeVolumes(&ec2.DescribeVolumesInput{
		VolumeIds: []*string{
			aws.String(name),
		},
	})
	if err != nil {
		return err
	}
	if len(volumes.Volumes) != 1 {
		return errors.New("Cannot find EBS volume.")
	}

	// TODO: check that it's in the right region.

	return nil
}

func (d *ebsVolumeDriver) Create(name string) error {
	m, exists := d.volumes[name]
	if exists {
		// Docker won't always cleanly remove entries.  It's okay so long
		// as the target isn't already mounted by someone else.
		if m != "" {
			return errors.New("Name already in use.")
		}
	}

	d.volumes[name] = ""
	return nil
}

func (d *ebsVolumeDriver) Mount(name string) (string, error) {
	m, exists := d.volumes[name]
	if !exists {
		return "", errors.New("Name not found.")
	}

	if m != "" {
		return "", errors.New("Volume already mounted.")
	}

	return d.doMount(name)
}

func (d *ebsVolumeDriver) Path(name string) (string, error) {
	m, exists := d.volumes[name]
	if !exists {
		return "", errors.New("Name not found.")
	}

	if m == "" {
		return "", errors.New("Volume not mounted.")
	}

	return m, nil
}

func (d *ebsVolumeDriver) Remove(name string) error {
	m, exists := d.volumes[name]
	if !exists {
		return errors.New("Name not found.")
	}

	// If the volume is still mounted, unmount it before removing it.
	if m != "" {
		err := d.doUnmount(name)
		if err != nil {
			return err
		}
	}

	delete(d.volumes, name)
	return nil
}

func (d *ebsVolumeDriver) Unmount(name string) error {
	m, exists := d.volumes[name]
	if !exists {
		return errors.New("Name not found.")
	}

	// If the volume is mounted, go ahead and unmount it.  Ignore requests
	// to unmount volumes that aren't actually mounted.
	if m != "" {
		err := d.doUnmount(name)
		if err != nil {
			return err
		}
	}

	return nil
}

func (d *ebsVolumeDriver) doMount(name string) (string, error) {
	m := "/not_yet_implemented"
	d.volumes[name] = m
	return m, nil
}

func (d *ebsVolumeDriver) doUnmount(name string) error {
	d.volumes[name] = ""
	return nil
}
