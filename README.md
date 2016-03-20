# Blocker

Blocker is a ridiculously simple stateless volume plugin for Docker.  It makes
using [Amazon Elastic Block Store](https://aws.amazon.com/ebs/) volumes trivial,
a pretty useful thing for microservices like databases.

Blocker was designed to work with [Docker](https://docs.docker.com/)
[Machine](https://docs.docker.com/machine/) and
[Swarm](https://docs.docker.com/swarm/) and does as little as possible.  It
mounts and unmounts volumes, and that's about it.

To make an EBS volume accessible to a container, pass `--volume-driver blocker`
and a `-v` volume mount specification to the `docker run` command:

    docker run \
        --volume-driver blocker \
        -v <ebs-volume-id>:<container-path> \
        ...

In this example, `<ebs-volume-id>` is the EBS volume identifier, typically in
the form `vol-00000000`, and `<container-path>` is the path within the
container at which the volume will be mounted.

For example, to run a MongoDB container with a persistent volume `vol-933e6c67`,
run this:

    docker run \
        --volume-driver blocker \
        -v vol-933e6c67:/data/db \
        mongo

EBS volumes must be properly initialized before using them.  Per Blocker's
stance on simplicity, it doesn't attempt to do anything fancy here.  This likely
entails creating a filesystem, for example, since EBS creates blank volumes by
default.  See [this handy guide](
http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ebs-using-volumes.html) for
more details on how to do this.

The target volume must be in the same AWS region and availability zone as the
machine running Docker.  Blocker will print these out when it starts up.  The
daemon will automatically attach and detach volumes as necessary.

## Installation

To install Blocker, just run this on the host running Docker:

    wget -qO- \
        https://raw.githubusercontent.com/joeduffy/blocker/master/res/get.sh | sh

If you're running Docker Swarm, you'll want to run this on the master and agents.

The install script installs an Upstart service named `blocker` whose output is
logged to the `/var/log/upstart/blocker.log` file.  If all has gone well you'll
see information something like this:

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

## Known issues

#####`Error response from daemon: 400 Bad Request: malformed Host header` 
For older docker version blocker must be built using golang 1.5. This could be done with the following command

    docker run \
        --rm \
        -v "$PWD":/usr/src/myapp \
        -w /usr/src/myapp \
        golang:1.5 sh -c 'go get -v; go build -v -o blocker'

Related issues: [docker#20865](https://github.com/docker/docker/issues/20865), [rexray#317](https://github.com/emccode/rexray/issues/317)

#####`Error response from daemon: no such file or directory`
May happen when docker is running with enabled selinux. As a workadund disable selinux for particular container with `--security-opt=label:disable`

e.g.

    docker run \
        --security-opt=label:disable \
        --volume-driver blocker \
        -v vol-933e6c67:/data/db \
        mongo
        
Related issues: [docker:#18005](https://github.com/docker/docker/issues/18005)
