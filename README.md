# Hotdog

Hotdog is a set of OCI hooks used to inject the
[Log4j Hot Patch](https://github.com/corretto/hotpatch-for-apache-log4j2) into
containers.

:warning: Hotdog is very close to end-of-life.
It has been twenty months since CVE-2021-44228 was discovered, and we expect that the vast majority of Java applications have been patched by this time.
We're also not aware of any use of Hotdog outside of Bottlerocket, which no longer uses it.
Therefore, we plan to end-of-life Hotdog by November 2023.
Please [open an issue](https://github.com/bottlerocket-os/hotdog/issues/new) if this affects you.


## How it works

When runc sets up the container, it invokes `hotdog-cc-hook`.  `hotdog-cc-hook`
bind-mounts the hotpatch files into the container's filesystem at
`/dev/shm/.hotdog`.  After the main container process starts, runc invokes
`hotdog-poststart-hook`, which uses `nsenter` to enter the container's
namespaces and fork off a `hotdog-hotpatch` process.  `hotdog-hotpatch` runs
several times with decreasing frequency (currently 1s, 5s, 10s, 30s) to detect
and hotpatch JVMs inside the container.

## Limitations

* Hotdog only provides hotpatching support for Java 8, 11, 15, and 17.
* Hotdog only runs for a short time at the beginning of a container's lifetime.
  If new Java processes are started after the `hotdog-hotpatch` process exits,
  they will not be hot patched.
* Hotdog only patches processes named "java".  If your Java application has a
  different process name, hotdog will not patch it.
* Hotdog works best when the container has its own pid namespace.  If hotdog is
  used with a container that has a shared pid namespace, the `hotdog-hotpatch`
  might remain for a short time after the container exits.
* Hotdog injects its components into `/dev/shm/.hotdog` inside the container.
  If `/dev/shm` does not exist (such as in the case of Docker containers
  launched with `--ipc=none`), hotdog will not be injected into the container
  and will not provide hotpatching.

## Installation

### Bottlerocket

Hotdog is included by default in Bottlerocket 1.5.0.

Hotpatching can be enabled for new launches of Bottlerocket by including the
following settings in user data.

```toml
[settings.oci-hooks]
log4j-hotpatch-enabled = true
```

For existing hosts running the latest version of Bottlerocket, hotpatching can
be enabled using the API client.

```shell
apiclient set oci-hooks.log4j-hotpatch-enabled=true
```

Enabling the setting at runtime has no effect on running containers.
Newly-launched containers will be hotpatched.

### Other Linux distributions

To install Hotdog, you need to copy the following files to the right location
and set the appropriate configuration.

* Copy `Log4jHotPatch.jar` to `/usr/share/hotdog` (if you build the hotpatch
  from source, you'll find it in `build/libs`)
* Run `make && sudo make install` to install `hotdog-cc-hook` and
  `hotdog-poststart-hook` to `/usr/libexec/hotdog` and `hotdog-hotpatch` to
  `/usr/share/hotdog`
* Install [`oci-add-hooks`](https://github.com/awslabs/oci-add-hooks/)
* Configure `oci-add-hooks` with the hotdog hooks by writing the following
  contents to `/etc/hotdog/config.json`:
  ```json
  {
    "hooks": {
      "prestart": [{
        "path": "/usr/libexec/hotdog/hotdog-cc-hook"
      }],
      "poststart": [{
        "path": "/usr/libexec/hotdog/hotdog-poststart-hook"
      }]
    }
  }
  ```
* Configure Docker to use the hooks by writing the following contents into
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

To run a container with hotpatching enabled, specify
`docker run --runtime hotdog`.  To run with hotpatching enabled by default in
all containers, add the following contents to `/etc/docker/daemon.json`:
```
"default-runtime": "hotdog"
```
If you wish to opt-out of `hotdog` even when it is enabled by default, specify
`--runtime runc`.

## Troubleshooting

`hotdog` will add several files to the `/dev/shm/.hotdog` directory in each
container.  You can find the log from `hotdog-hotpatch` in
`/dev/shm/hotdog.log`.

## Security

See [CONTRIBUTING](CONTRIBUTING.md#security-issue-notifications) for more information.

## License

This project is licensed under the Apache-2.0 License.
