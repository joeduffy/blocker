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
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/satori/go.uuid"
)

type ebsVolume struct {
	name     string
	mount    string
	volumeId string
}

type ebsVolumeDriver struct {
	ec2                 *ec2.EC2
	ec2meta             *ec2metadata.EC2Metadata
	awsInstanceId       string
	awsRegion           string
	awsAvailabilityZone string
	volumes             map[string]*ebsVolume
}

func NewEbsVolumeDriver() (VolumeDriver, error) {
	d := &ebsVolumeDriver{
		volumes: make(map[string]*ebsVolume),
	}

	ec2sess := session.New()
	d.ec2meta = ec2metadata.New(ec2sess)

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

	d.ec2 = ec2.New(ec2sess, &aws.Config{Region: aws.String(d.awsRegion)})

	// Print some diagnostic information and then return the driver.
	log("Auto-detected EC2 information:\n")
	log("\tInstanceId        : %v\n", d.awsInstanceId)
	log("\tRegion            : %v\n", d.awsRegion)
	log("\tAvailability Zone : %v\n", d.awsAvailabilityZone)
	return d, nil
}

func (d *ebsVolumeDriver) Create(name string, opts map[string]string) error {

	vol, exists := d.volumes[name]
	if exists {
		// Docker won't always cleanly remove entries.  It's okay so long
		// as the target isn't already mounted by someone else.
		if vol.mount != "" {
			return errors.New("Name already in use.")
		}
	} else {
		// Create a new volume, defaulting the ID to its name, and add it to the map.
		volumeId := name
		serviceName, exists := opts["service"]
		if exists {
			v, err := d.getAvailableVolumeForService(serviceName)
			if err != nil {
				return err
			}
			volumeId = *v.VolumeId
		}

		vol = &ebsVolume{name: name, mount: "", volumeId: volumeId}
		d.volumes[name] = vol
	}

	// If a volume ID was given as an override, use it.
	volumeId, exists := opts["volume_id"]
	if exists {
		vol.volumeId = volumeId
	}

	return nil
}

func (d *ebsVolumeDriver) Mount(name string) (string, error) {
	vol, exists := d.volumes[name]
	if !exists {
		return "", errors.New("Name not found.")
	}

	if vol.mount != "" {
		return "", errors.New("Volume already mounted.")
	}

	return d.doMount(vol)
}

func (d *ebsVolumeDriver) Path(name string) (string, error) {
	vol, exists := d.volumes[name]
	if !exists {
		return "", errors.New("Name not found.")
	}

	if vol.mount == "" {
		return "", errors.New("Volume not mounted.")
	}

	return vol.mount, nil
}

func (d *ebsVolumeDriver) Remove(name string) error {
	vol, exists := d.volumes[name]
	if !exists {
		return errors.New("Name not found.")
	}

	// If the volume is still mounted, unmount it before removing it.
	if vol.mount != "" {
		err := d.doUnmount(vol)
		if err != nil {
			return err
		}
	}

	delete(d.volumes, name)
	return nil
}

func (d *ebsVolumeDriver) Unmount(name string) error {
	vol, exists := d.volumes[name]
	if !exists {
		return errors.New("Name not found.")
	}

	// If the volume is mounted, go ahead and unmount it.  Ignore requests
	// to unmount volumes that aren't actually mounted.
	if vol.mount != "" {
		err := d.doUnmount(vol)
		if err != nil {
			return err
		}
	}

	return nil
}

func (d *ebsVolumeDriver) doMount(vol *ebsVolume) (string, error) {
	// Auto-generate a random mountpoint.
	mount := "/mnt/blocker/" + uuid.NewV4().String()

	// Ensure the directory /mnt/blocker/<m> exists.
	if err := os.MkdirAll(mount, os.ModeDir|0700); err != nil {
		return "", err
	}
	if stat, err := os.Stat(mount); err != nil || !stat.IsDir() {
		return "", fmt.Errorf("Mountpoint %v is not a directory: %v", mount, err)
	}

	// Attach the EBS device to the current EC2 instance.
	dev, err := d.attachVolume(vol)
	if err != nil {
		return "", err
	}

	// Now go ahead and mount the EBS device to the desired mountpoint.
	// TODO: support encrypted filesystems.
	if out, err := exec.Command("mount", dev, mount).CombinedOutput(); err != nil {
		// Make sure to detach the instance before quitting (ignoring errors).
		d.detachVolume(vol)

		return "", fmt.Errorf("Mounting device %v to %v failed: %v\n%v",
			dev, mount, err, string(out))
	}

	// Set the volume's mountpoint and return it.
	vol.mount = mount
	return mount, nil
}

func (d *ebsVolumeDriver) waitUntilState(
	volumeId string, check func(*ec2.Volume) error) error {
	// Most volume operations are asynchronous, and we often need to wait until
	// state transitions finish before proceeding to the mount.  Sadly, this
	// requires some clunky retries, sleeps, and that kind of crap.
	tries := 0
	for {
		tries++

		volumes, err := d.ec2.DescribeVolumes(&ec2.DescribeVolumesInput{
			VolumeIds: []*string{aws.String(volumeId)},
		})
		if err != nil {
			return err
		}

		// Check to see if the volume reached the intended state; if yes, return.
		err = check(volumes.Volumes[0])
		if err == nil {
			return nil
		}
		if tries == 12 {
			return err
		}

		log("\tWaiting for EBS attach to complete...\n")
		time.Sleep(5 * time.Second)
	}

	return nil
}

func (d *ebsVolumeDriver) waitUntilAttached(volumeId string) error {
	return d.waitUntilState(volumeId, func(volume *ec2.Volume) error {
		var attachment *ec2.VolumeAttachment
		if len(volume.Attachments) == 1 {
			attachment = volume.Attachments[0]
			if *attachment.State == ec2.VolumeAttachmentStateAttached {
				return nil
			}
		}
		if attachment == nil {
			return fmt.Errorf(
				"Volume state transition failed: expected 1 attachment, got %v",
				len(volume.Attachments))
		} else {
			return fmt.Errorf(
				"Volume state transition failed: seeking %v, current is %v",
				ec2.VolumeAttachmentStateAttached, *attachment.State)
		}
	})
}

func (d *ebsVolumeDriver) waitUntilAvailable(volumeId string) error {
	return d.waitUntilState(volumeId, func(volume *ec2.Volume) error {
		if *volume.State == ec2.VolumeStateAvailable {
			return nil
		}
		return fmt.Errorf(
			"Volume state transition failed: seeking %v, current is %v",
			ec2.VolumeStateAvailable, *volume.State)
	})
}

func (d *ebsVolumeDriver) attachVolume(vol *ebsVolume) (string, error) {
	// Since detaching is asynchronous, we want to check first to see if the
	// target volume is in the process of being detached.  If it is, we'll wait
	// a little bit until it's ready to use.
	err := d.waitUntilAvailable(vol.volumeId)
	if err != nil {
		return "", err
	}

	// Now find the first free device to attach the EBS volume to.  See
	// http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/device_naming.html
	// for recommended naming scheme (/dev/sd[f-p]).
	for _, c := range "fghijklmnop" {
		dev := "/dev/sd" + string(c)
		altdev := "/dev/xvd" + string(c)

		if _, err := os.Lstat(dev); err == nil {
			continue
		}
		if _, err := os.Lstat(altdev); err == nil {
			continue
		}

		if _, err := d.ec2.AttachVolume(&ec2.AttachVolumeInput{
			Device:     aws.String(dev),
			InstanceId: aws.String(d.awsInstanceId),
			VolumeId:   aws.String(vol.volumeId),
		}); err != nil {
			if awsErr, ok := err.(awserr.Error); ok &&
				awsErr.Code() == "InvalidParameterValue" {
				// If AWS is simply reporting that the device is already in
				// use, then go ahead and check the next one.
				continue
			}

			return "", err
		}

		err = d.waitUntilAttached(vol.volumeId)
		if err != nil {
			return "", err
		}

		// Finally, the attach is complete.
		log("\tAttached EBS volume name=%v/volumeId=%v to %v:%v.\n",
			vol.name, vol.volumeId, d.awsInstanceId, dev)
		if _, err := os.Lstat(dev); os.IsNotExist(err) {
			// On newer Linux kernels, /dev/sd* is mapped to /dev/xvd*.  See
			// if that's the case.
			if _, err := os.Lstat(altdev); os.IsNotExist(err) {
				d.detachVolume(vol)
				return "", fmt.Errorf("Device %v is missing after attach.", dev)
			}

			log("\tLocal device name is %v\n", altdev)
			dev = altdev
		}

		return dev, nil
	}

	return "", errors.New("No devices available for attach: /dev/sd[f-p] taken.")
}

func (d *ebsVolumeDriver) doUnmount(vol *ebsVolume) error {
	// First unmount the device.
	if out, err := exec.Command("umount", vol.mount).CombinedOutput(); err != nil {
		return fmt.Errorf("Unmounting %v failed: %v\n%v", vol.mount, err, string(out))
	}

	// Remove the mountpoint from the filesystem.
	if err := os.Remove(vol.mount); err != nil {
		return err
	}

	// Detach the EBS volume from this AWS instance.
	if err := d.detachVolume(vol); err != nil {
		return err
	}

	// Clear out the mount information and return.
	vol.mount = ""
	return nil
}

func (d *ebsVolumeDriver) detachVolume(vol *ebsVolume) error {
	if _, err := d.ec2.DetachVolume(&ec2.DetachVolumeInput{
		InstanceId: aws.String(d.awsInstanceId),
		VolumeId:   aws.String(vol.volumeId),
	}); err != nil {
		return err
	}

	log("\tDetached EBS volume name=%v/volumeId=%v from %v.\n",
		vol.name, vol.volumeId, d.awsInstanceId)
	return nil
}
