# `gcsfs`

[![Travis CI](https://api.travis-ci.com/ofek/docker-volume-gcs.svg?branch=master)](https://travis-ci.com/ofek/docker-volume-gcs)
[![Docker - Pulls](https://img.shields.io/docker/pulls/ofekmeister/gcsfs.svg)](https://hub.docker.com/r/ofekmeister/gcsfs)
[![License - MIT/Apache-2.0](https://img.shields.io/badge/license-MIT%2FApache--2.0-9400d3.svg)](https://choosealicense.com/licenses)
[![Say Thanks](https://img.shields.io/badge/say-thanks-ff69b4.svg)](https://saythanks.io/to/ofek)

-----

An easy-to-use, cross-platform, and highly optimized Docker Volume Plugin for mounting Google Cloud Storage buckets.

**Table of Contents**

- [Installation](#installation)
- [Usage](#usage)
  - [Standard](#standard)
  - [Key mounting](#key-mounting)
- [Driver options](#driver-options)
- [Permission](#permission)
- [License](#license)
- [Credits](#credits)
- [Future](#future)

## Installation

`gcsfs` is distributed on [Docker Hub](https://hub.docker.com/search), allowing a seamless install:

```console
$ docker plugin install ofekmeister/gcsfs
```

You will also need at least one [service account key](#permission).

## Usage

### Standard

Create a volume with the key contents:

```console
$ docker volume create -d ofekmeister/gcsfs -o key=$(cat service_account_key_file) <BUCKET_NAME>
```

or via `docker-compose`:

```yaml
version: '3.4'

volumes:
  mybucket:
    name: <BUCKET_NAME>
    driver: ofekmeister/gcsfs
    driver_opts:
      key: ${KEY_CONTENTS_IN_ENV_VAR}
```

Then create a container that uses the volume:

```console
$ docker run -v <BUCKET_NAME>:/data --rm -d --name gcsfs-test alpine tail -f /dev/null
```

or via `docker-compose`:

```yaml
services:
  test:
    container_name: gcsfs-test
    image: alpine
    entrypoint: ['tail', '-f', '/dev/null']
    volumes:
    - mybucket:/data
```

At this point you should be able to access your bucket:

```console
$ docker exec gcsfs-test ls /data
```

### Key mounting

Alternatively, you can mount a directory of service account keys and reference the file name.

First disable the plugin:

```console
$ docker plugin disable ofekmeister/gcsfs
```

then set the `keys.source` option:

```console
$ docker plugin set ofekmeister/gcsfs keys.source=/path/to/keys
```

If you don't yet have the plugin, this can also be done during the installation:

```console
$ docker plugin install ofekmeister/gcsfs keys.source=/path/to/keys
```

**Note:** On Windows you'll need to use `host_mnt` paths e.g. `C:\path\to\keys` would become `/host_mnt/c/path/to/keys`.

Assuming there is a file named `credentials.json` in `/path/to/keys`, you can now create a volume by doing:

```console
$ docker volume create -d ofekmeister/gcsfs -o key=credentials.json <BUCKET_NAME>
```

or via `docker-compose`:

```yaml
version: '3.4'

volumes:
  mybucket:
    name: <BUCKET_NAME>
    driver: ofekmeister/gcsfs
    driver_opts:
      key: credentials.json
```

## Driver options

- `key` - The file name of the key in the `keys.source` directory, or else the raw key contents if it doesn't exist.
- `bucket` - The Google Cloud Storage bucket to use. If this is not specified, the volume name is assumed to be the desired bucket.
- `flags` - Extra [flags](https://github.com/ofek/docker-volume-gcs/blob/master/gcsfuse_flags) to
  pass to [gcsfuse][1] e.g. `-o flags="--limit-ops-per-sec=10 --only-dir=some/nested/folder"`.
- `debug` - A timeout (in seconds) used only for testing. This will attempt to mount the bucket, wait for logs, then un-mount and print debug info.

## Permission

In order to access anything stored in Google Cloud Storage, you will need service accounts with appropriate IAM
roles. Read more about them [here](https://cloud.google.com/iam/docs/understanding-service-accounts). If writes
are needed, you will usually select `roles/storage.admin` scoped to the desired buckets.

The easiest way to create service account keys, if you don't yet have any, is to run:

```console
$ gcloud iam service-accounts list
```

to find the email of a desired service account, then run:

```console
$ gcloud iam service-accounts keys create <FILE_NAME>.json --iam-account <EMAIL>
```

to create a key file.

**Tip:** If you have a service account with write access you want to share with containers that should only
be able to read, you can append the standard `:ro` to avoid creating a new read-only service account.

## License

`gcsfs` is distributed under the terms of both

- [Apache License, Version 2.0](https://choosealicense.com/licenses/apache-2.0)
- [MIT License](https://choosealicense.com/licenses/mit)

at your option.

## Credits

- [gcsfuse](1)
- [Brian Goff](https://github.com/cpuguy83) and [Justin Cormack](https://github.com/justincormack) for being helpful in Slack
  and encouraging me to write this to overcome a [limitation on non-Linux hosts](https://github.com/moby/moby/issues/39093).

## Future

I also want to make a [Kubernetes CSI driver](https://kubernetes-csi.github.io/docs). However, that
won't happen for a while as it appears to me I'll need to learn everything about everything.

[1]: https://github.com/GoogleCloudPlatform/gcsfuse
