module github.com/bottlerocket/hotdog

go 1.19

require (
	github.com/opencontainers/runtime-spec v1.0.2
	github.com/opencontainers/selinux v1.10.1
	golang.org/x/sys v0.0.0-20220919091848-fb04ddd9f9c8
	kernel.org/pub/linux/libs/security/libcap/cap v1.2.65
)

require kernel.org/pub/linux/libs/security/libcap/psx v1.2.65 // indirect
