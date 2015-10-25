package main

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"code.google.com/p/go-uuid/uuid"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/service/ec2"
)

type ebsVolumeDriver struct {
	ec2                 *ec2.EC2
	ec2meta             *ec2metadata.Client
	awsInstanceId       string
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
	d.awsInstanceId, err = ec2meta.GetMetadata("instance-id")
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
	// First, attach the EBS device to the current EC2 instance.
	dev, err := d.attachVolume(name)
	if err != nil {
		return "", err
	}

	// Now auto-generate a random mountpoint.
	mnt := "/mnt/blocker" + uuid.New()

	// Ensure the directory /mnt/blocker/<m> exists.
	if err := os.MkdirAll(mnt, os.ModeDir|0700); err != nil {
		return "", err
	}
	if stat, err := os.Stat(mnt); err != nil || !stat.IsDir() {
		return "", errors.New("Mountpoint creation failed.")
	}

	// Now go ahead and mount the EBS device to the desired mountpoint.
	// TODO: permit the filesystem type in the name.
	// TODO: detect and auto-format unformatted filesystems.
	if err := syscall.Mount(dev, mnt, "ext4", 0, ""); err != nil {
		return "", err
	}

	// And finally set and return it.
	d.volumes[name] = mnt
	return mnt, nil
}

func (d *ebsVolumeDriver) attachVolume(name string) (string, error) {
	dev, err := d.findFreeDeviceForAttach()
	if err != nil {
		return "", err
	}

	if _, err := d.ec2.AttachVolume(&ec2.AttachVolumeInput{
		Device:     aws.String(dev),
		InstanceId: aws.String(d.awsInstanceId),
		VolumeId:   aws.String(name),
	}); err != nil {
		return "", err
	}

	fmt.Printf("Attached EBS volume %v to %v:%v.", name, d.awsInstanceId, dev)
	return dev, nil
}

func (d *ebsVolumeDriver) findFreeDeviceForAttach() (string, error) {
	// Get a list of instance devices.
	volumes, err := d.ec2.DescribeVolumes(&ec2.DescribeVolumesInput{
		Filters: []*ec2.Filter{
			{
				Name: aws.String("attachment.instance-id"),
				Values: []*string{
					aws.String(d.awsInstanceId),
				},
			},
		},
	})
	if err != nil {
		return "", err
	}

	devices := make(map[string]bool)
	for _, volume := range volumes.Volumes {
		if len(volume.Attachments) == 0 {
			continue
		}
		devices[*volume.Attachments[0].Device] = true
	}

	// Now find the first free devices to attach the EBS volume.  See
	// http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/device_naming.html
	// for recommended naming scheme (/dev/sd[f-p]).
	for _, c := range "fghijklmnop" {
		dev := "/dev/sd" + string(c)
		if _, exists := devices[dev]; !exists {
			return dev, nil
		}
	}

	return "", errors.New("No devices available for attach: /dev/sd[f-p] taken.")
}

func (d *ebsVolumeDriver) doUnmount(name string) error {
	mnt := d.volumes[name]

	// First unmount the device.
	if err := syscall.Unmount(mnt, 0); err != nil {
		return err
	}

	// Remove the mountpoint from the filesystem.
	if err := os.Remove(mnt); err != nil {
		return err
	}

	// Detach the EBS volume from this AWS instance.
	if err := d.detachVolume(name); err != nil {
		return err
	}

	// Finally clear out the slot and return.
	d.volumes[name] = ""
	return nil
}

func (d *ebsVolumeDriver) detachVolume(name string) error {
	if _, err := d.ec2.DetachVolume(&ec2.DetachVolumeInput{
		InstanceId: aws.String(d.awsInstanceId),
		VolumeId:   aws.String(name),
	}); err != nil {
		return err
	}

	fmt.Printf("Detached EBS volume %v from %v.", name, d.awsInstanceId)
	return nil
}
