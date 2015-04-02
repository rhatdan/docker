package configs

type Mount struct {
	// Source path for the mount.
	Source string `json:"source"`

	// Destination path for the mount inside the container.
	Destination string `json:"destination"`

	// Device the mount is for.
	Device string `json:"device"`

	// Mount flags.
	Flags int `json:"flags"`

	// Mount data applied to the mount.
	Data string `json:"data"`

	// Relabel source if set, "z" indicates shared, "Z" indicates unshared.
	Relabel string `json:"relabel"`

	// Optional Command to be run before Source is mounted.
	PremountCmd [][]string `json:"premountcmd"`

	// Optional Command to be run after Source is mounted.
	PostmountCmd [][]string `json:"postmountcmd"`
}
