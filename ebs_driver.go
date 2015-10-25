package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/satori/go.uuid"
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
	d := &ebsVolumeDriver{
		volumes: make(map[string]string),
	}
	d.ec2meta = ec2metadata.New(nil)

	// Fetch AWS information, validating along the way.
	if !d.ec2meta.Available() {
		return nil, errors.New("Not running on an EC2 instance.")
	}
	var err error
	if d.awsInstanceId, err = d.ec2meta.GetMetadata("instance-id"); err != nil {
		return nil, err
	}
	if d.awsRegion, err = d.ec2meta.Region(); err != nil {
		return nil, err
	}
	if d.awsAvailabilityZone, err =
		d.ec2meta.GetMetadata("placement/availability-zone"); err != nil {
		return nil, err
	}

	d.ec2 = ec2.New(aws.NewConfig().WithRegion(d.awsRegion))

	// Print some diagnostic information and then return the driver.
	log("Auto-detected EC2 information:\n")
	log("\tInstanceId        : %v\n", d.awsInstanceId)
	log("\tRegion            : %v\n", d.awsRegion)
	log("\tAvailability Zone : %v\n", d.awsAvailabilityZone)
	return d, nil
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
	// Auto-generate a random mountpoint.
	mnt := "/mnt/blocker/" + uuid.NewV4().String()

	// Ensure the directory /mnt/blocker/<m> exists.
	if err := os.MkdirAll(mnt, os.ModeDir|0700); err != nil {
		return "", err
	}
	if stat, err := os.Stat(mnt); err != nil || !stat.IsDir() {
		return "", fmt.Errorf("Mountpoint %v is not a directory: %v", mnt, err)
	}

	// Attach the EBS device to the current EC2 instance.
	dev, err := d.attachVolume(name)
	if err != nil {
		return "", err
	}

	// Now go ahead and mount the EBS device to the desired mountpoint.
	// TODO: support encrypted filesystems.
	if out, err := exec.Command("mount", dev, mnt).CombinedOutput(); err != nil {
		// Make sure to detach the instance before quitting (ignoring errors).
		d.detachVolume(name)

		return "", fmt.Errorf("Mounting device %v to %v failed: %v\n%v",
			dev, mnt, err, string(out))
	}

	// And finally set and return it.
	d.volumes[name] = mnt
	return mnt, nil
}

func (d *ebsVolumeDriver) attachVolume(name string) (string, error) {
	// Now find the first free device to attach the EBS volume to.  See
	// http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/device_naming.html
	// for recommended naming scheme (/dev/sd[f-p]).
	for _, c := range "fghijklmnop" {
		dev := "/dev/sd" + string(c)

		// TODO: we could check locally first to eliminate a few network
		//     roundtrips in the event that some devices are used.  Even if we
		//     did that, however, we'd need the checks regarding the AWS request
		//     failing below, because of TOCTOU.

		if _, err := d.ec2.AttachVolume(&ec2.AttachVolumeInput{
			Device:     aws.String(dev),
			InstanceId: aws.String(d.awsInstanceId),
			VolumeId:   aws.String(name),
		}); err != nil {
			if awsErr, ok := err.(awserr.Error); ok &&
				awsErr.Code() == "InvalidParameterValue" {
				// If AWS is simply reporting that the device is already in
				// use, then go ahead and check the next one.
				continue
			}

			return "", err
		}

		// The attach operation is asynchronous, so we now need to wait until
		// it completes before proceeding to the mount.  Sadly, this requires
		// some clunky retries, sleeps, and that kind of crap.
		tries := 0
		for {
			tries++

			volumes, err := d.ec2.DescribeVolumes(&ec2.DescribeVolumesInput{
				VolumeIds: []*string{aws.String(name)},
			})
			if err != nil {
				return "", err
			}

			volume := volumes.Volumes[0]
			var attachment *ec2.VolumeAttachment
			if len(volume.Attachments) == 1 {
				attachment = volume.Attachments[0]
				if *attachment.State == ec2.VolumeAttachmentStateAttached {
					// Good to go, we're all attached.
					break
				}
			}

			if tries == 5 {
				if attachment == nil {
					return "", fmt.Errorf(
						"Volume attach failed: expected 1 attachment, got %v",
						len(volume.Attachments))
				} else {
					return "", fmt.Errorf(
						"Volume attach failed: state is %v", *attachment.State)
				}
			}

			log("\tWaiting for EBS attach to complete...\n")
			time.Sleep(5 * time.Second)
		}

		// Finally, the attach is complete.
		log("\tAttached EBS volume %v to %v:%v.\n", name, d.awsInstanceId, dev)
		if _, err := os.Lstat(dev); os.IsNotExist(err) {
			// On newer Linux kernels, /dev/sd* is mapped to /dev/xvd*.  See
			// if that's the case.
			altdev := "/dev/xvd" + string(c)
			if _, err := os.Lstat(altdev); os.IsNotExist(err) {
				d.detachVolume(name)
				return "", fmt.Errorf("Device %v is missing after attach.", dev)
			}

			log("\tLocal device name is %v\n", altdev)
			dev = altdev
		}

		return dev, nil
	}

	return "", errors.New("No devices available for attach: /dev/sd[f-p] taken.")
}

func (d *ebsVolumeDriver) doUnmount(name string) error {
	mnt := d.volumes[name]

	// First unmount the device.
	if out, err := exec.Command("umount", mnt).CombinedOutput(); err != nil {
		return fmt.Errorf("Unmounting %v failed: %v\n%v", mnt, err, string(out))
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

	log("\tDetached EBS volume %v from %v.\n", name, d.awsInstanceId)
	return nil
}
