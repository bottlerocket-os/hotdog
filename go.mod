module github.com/bottlerocket/hotdog

go 1.19

require (
	github.com/opencontainers/runtime-spec v1.0.2
	github.com/opencontainers/selinux v1.11.0
	golang.org/x/sys v0.5.0
	kernel.org/pub/linux/libs/security/libcap/cap v1.2.67
)

require kernel.org/pub/linux/libs/security/libcap/psx v1.2.67 // indirect
