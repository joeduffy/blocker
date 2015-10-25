package main

// Docker volume plugins enable Docker deployments to be integrated with
// external storage systems, and enable data volumes to persist beyond the
// lifetime of a single Docker host.  See the Docker plugin documentation for
// more information: https://docs.docker.com/extend/plugins_volume/
type VolumeDriver interface {
	// Instructs the plugin about a new volume.  The plugin need not actually
	// manifest the volume on the filesystem yet, until Mount is called.
	Create(name string) error

	// Mounts a volume, returning its mountpoint on the host filesystem.
	Mount(name string) (string, error)

	// Fetches the host mountpoint location for an existing volume.
	Path(name string) (string, error)

	// Removes an existing volume.
	Remove(name string) error

	// Unmounts an existing volume.
	Unmount(name string) error
}
