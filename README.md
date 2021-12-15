# Hotdog

Hotdog is a set of OCI hooks used to inject the
[Log4j Hot Patch](https://github.com/corretto/hotpatch-for-apache-log4j2) into
containers.

## Installation

To install Hotdog, you need to copy the following files to the right location
and set the appropriate configuration.

* Copy `jdk8-Log4jHotPatch.jar` to `/usr/libexec/hotdog`
* Copy `jdk11-Log4jHotPatch.jar` to `/usr/libexec/hotdog`
* Copy `jdk17-Log4jHotPatchFat.jar` to `/usr/libexec/hotdog`
* Run `make install` to install `hotdog-cc-hook`, `hotdog-poststart-hook`, and
  `hotdog-poststop-hook` to `/usr/local/bin` and to install `hotdog-hotpatch`
  to `/usr/libexec/hotdog`
* Install `oci-add-hooks`
* Configure `oci-add-hooks` as by writing the following contents to
  `/etc/hotdog/config.json`:
  ```json
  {
    "hooks": {
      "prestart": [{
        "path": "/usr/local/bin/hotdog-cc-hook"
      }],
      "poststart": [{
        "path": "/usr/local/bin/hotdog-poststart-hook"
      }],
      "poststop": [{
        "path": "/usr/local/bin/hotdog-poststop-hook"
      }]
    }
  }
  ```
* Configure Docker to use the hook by writing the following contents into
  `/etc/docker/daemon.json`:
  ```json
  {
    "runtimes": {
      "hotdog": {
        "path": "oci-add-hooks",
        "runtimeArgs": [
          "--hook-config-path", "/etc/hotdog/config.json",
          "--runtime-path", "/usr/sbin/runc"
        ]
      }
    }
  }
  ```

To run a container with the hotpatching enabled, specify
`docker run --runtime hotdog`.  To run with hotpatching enabled by default in
all containers, add the following contents to `/etc/docker/daemon.json`:
```
"default-runtime": "hotdog"
```
If you wish to opt-out of `hotdog` even when it is enabled by default, specify
`--runtime runc`.

## How it works

When runc sets up the container, it invokes `hotdog-cc-hook`.  `hotdog-cc-hook`
copies the hotpatch files into the container's filesystem at `/.hotdog`.  After
the main container process starts, runc invokes `hotdog-poststart-hook`, which
uses `nsenter` to enter the container's namespaces and fork off a
`hotdog-hotpatch` process.  `hotdog-hotpatch` runs several times with
decreasing frequency (currently 1s, 5s, 10s, 30s) to detect and hotpach JVMs
inside the container.

## Troubleshooting

`hotdog` will add several files to the `/.hotdog` directory in each container.
You can find the log from `hotdog-hotpatch` in `/.hotdog/hotdog.log`.