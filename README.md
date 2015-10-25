# Blocker

Blocker is a ridiculously simple stateless volume plugin for Docker.  It makes
using [Amazon Elastic Block Store](https://aws.amazon.com/ebs/) volumes trivial,
a pretty useful thing for microservices like databases:

    docker run -v <ebs-volume-id>:<container-path> --volume-driver blocker ...

Blocker was designed to work with [Docker](https://docs.docker.com/)
[Machine](https://docs.docker.com/machine/) and
[Swarm](https://docs.docker.com/swarm/) and does as little as possible.  It
mounts and unmounts volumes, and that's about it.

To install Blocker, just run this on the host running Docker:

    wget -qO- \
        https://raw.githubusercontent.com/joeduffy/blocker/master/res/get.sh | sh

If you're running Docker Swarm, you'll want to run this on the master and agents.

Once installed, you can mount a volume in the usual Docker style, by passing
`--volume-driver blocker` and specifying your EBS volume ID as the name.  E.g.:

    docker run -v vol-933e6c67:/test --volume-driver blocker ...

Once the container starts, the volume will be accessible inside at `/test`.

Volumes must be created and mounted prior to using them.  Per Blocker's stance
on simplicity, it doesn't attempt to do anything fancy here.

The target volume must be in the same AWS region and availability zone as the
machine running Docker.  Blocker will print these out when it starts up.  The
daemon will automatically attach and detach volumes as necessary.

The install script installs an Upstart service named `blocker` whose output is
logged to the `/var/log/upstart/blocker.log` file.  If all has gone well you'll
see information something like the following:

    2015/10/25 18:07:11 blocker:starting up...
    2015/10/25 18:07:11 Auto-detected EC2 information:
    2015/10/25 18:07:11     InstanceId        : i-5bdf67b9
    2015/10/25 18:07:11     Region            : us-west-2
    2015/10/25 18:07:11     Availability Zone : us-west-2a
    2015/10/25 18:07:11 Ready to go; listening on socket /var/run/blocker.sock...

Additional information for all mounting and unmounting activities is logged.

**Note, AWS authentication information must be available before starting Blocker.**
See [this guide](https://github.com/aws/aws-sdk-go/wiki/Getting-Started-Credentials)
for details on how this is done.  In short, the easiest is to generate an
`~/.aws/credentials` file containing an `aws_access_key_id` and
`aws_secret_access_key`.  This is subtly different than what you get from
running `aws configure`, though it's very close.  An alternative is to export
`AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` environment variables, but this
is a bit tricky because the Upstart process needs access to them.

## Other Platforms

At present, only Linux x64 is supported as a host platform.  I am open to
supporting other hosts if you want to submit a pull request.

Note that supporting Mac OS X and Windows isn't necessary, since they run Docker
within a Docker Machine.  To use Blocker in these cases, merely provision a
machine -- let's say `default` -- and then SSH into it and run the installation
as specified above.  I.e.:

    $ docker machine create default --driver virtualbox
    $ docker machine ssh default
    <in the machine>
    $ wget -qO- ... installation shown earlier ...

## Other Cloud Storage Providers

I am not opposed to supporting cloud storage providers other than Amazon.  In
fact, the code is setup to do this fairly easily (the VolumeDriver interface
simply needs multiple implementations).  If you want to contribute this, I'm
happy to accept pull requests, so long as it doesn't complicate the original
intent of keeping this driver as simple as possible.

