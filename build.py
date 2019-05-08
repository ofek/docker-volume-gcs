import argparse
import os
import shutil
import subprocess
import sys
from os import path
from tempfile import TemporaryDirectory

VERSION = '1.0.0'
GCSFUSE_VERSION = '0.27.0'
BUILDER_IMAGE = 'docker:stable-dind'

HERE = path.dirname(path.abspath(__file__))
REPO = 'ofekmeister'
IMAGE = 'gcsfs'
PLUGIN_NAME = f'{REPO}/{IMAGE}'


class PluginBuilder:
    WORK_DIR = '/home'
    BUILD_DIR = '/tmp/build'
    ROOTFS_DIR = '/tmp/build/rootfs'

    BUILDER_NAME = f'{IMAGE}-builder'
    ROOTFS_IMAGE = f'{IMAGE}:rootfs'
    ROOTFS_NAME = f'{IMAGE}-rootfs'

    def __init__(self, builder_image, work_dir):
        self.builder_image = builder_image
        self.work_dir = work_dir
        self.origin = None
        self.current_stage = 0

    def stage(self, message):
        self.current_stage += 1

        if self.current_stage > 1:
            print()

        print(f'-----> Stage {self.current_stage}')
        print(f'       {message}')

    def run(self, command, **kwargs):
        self.stage(' '.join(command))
        return subprocess.run(command, **kwargs)

    def runi(self, command, **kwargs):
        cmd = ['docker', 'exec']

        if kwargs.pop('chdir', True):
            cmd.extend(('-w', self.WORK_DIR))

        if kwargs.pop('tty', False):
            cmd.append('-it')

        return self.run([*cmd, self.BUILDER_NAME, *command], **kwargs)

    def stop(self):
        self.run(['docker', 'rm', '-v', '-f', self.BUILDER_NAME])

    def __enter__(self):
        try:
            self.run(['docker', 'pull', self.builder_image], check=True)
            self.run(
                [
                    'docker', 'run', '-d', '--privileged',
                    '-v', f'{self.work_dir}:{self.WORK_DIR}',
                    '--name', self.BUILDER_NAME,
                    self.builder_image,
                ],
                check=True,
            )
        except Exception:
            self.stop()
            raise

        self.origin = os.getcwd()
        os.chdir(self.work_dir)
        return self

    def __exit__(self, exc_type, exc_value, traceback):
        os.chdir(self.origin)
        self.stop()


def main():
    parser = argparse.ArgumentParser(prog=f'{PLUGIN_NAME} builder')

    parser.add_argument('-n', '--no-push', action='store_true', help='Skip the publishing stage')
    parser.add_argument('-r', '--release', action='store_true', help='Maximize image size optimization and tag latest')
    parser.add_argument('-l', '--lint', action='store_true', help='Run linters')
    parser.add_argument('-t', dest='tag', type=str, default=VERSION, help=f'The desired tag (default: {VERSION})')
    parser.add_argument('-g', dest='gcsfuse', type=str, default=GCSFUSE_VERSION, help=f'The version or commit hash of gcsfuse (default: {GCSFUSE_VERSION})')
    parser.add_argument('-b', dest='builder', type=str, default=BUILDER_IMAGE, help=f'The builder image (default: {BUILDER_IMAGE})')

    args = parser.parse_args()

    releasing = args.release
    linting = str(args.lint).lower()
    image = f'{PLUGIN_NAME}:{args.tag}'
    image_latest = f'{PLUGIN_NAME}:latest'
    gcsfuse_version = args.gcsfuse
    if releasing:
        go_flags = '-ldflags=-s -ldflags=-w'
        upx_flags = '--best --ultra-brute'
    else:
        go_flags = ''
        upx_flags = '-1'
        image += '-rc'

    with TemporaryDirectory() as temp_dir:
        plugin_dir = path.join(path.realpath(temp_dir), 'plugin')
        shutil.copytree(HERE, plugin_dir, ignore=shutil.ignore_patterns('.git', 'vendor'), copy_function=shutil.copy)

        with PluginBuilder(args.builder, plugin_dir) as builder:
            result = builder.runi(['mkdir', '-p', builder.BUILD_DIR, builder.ROOTFS_DIR], chdir=False)
            if result.returncode:
                return result.returncode

            result = builder.runi(['cp', 'config.json', builder.BUILD_DIR])
            if result.returncode:
                return result.returncode

            result = builder.runi(
                [
                    'docker', 'build', '--no-cache',
                    '-f', 'plugin/Dockerfile',
                    '--tag', builder.ROOTFS_IMAGE,
                    '--build-arg', f'gcsfuse_version={gcsfuse_version}',
                    '--build-arg', f'go_flags={go_flags}',
                    '--build-arg', f'upx_flags={upx_flags}',
                    '--build-arg', f'lint={linting}',
                    'plugin'
                ]
            )
            if result.returncode:
                return result.returncode

            result = builder.runi(['docker', 'create', '--name', builder.ROOTFS_NAME, builder.ROOTFS_IMAGE])
            if result.returncode:
                return result.returncode

            result = builder.runi(['docker', 'export', '--output', 'fs.tar', builder.ROOTFS_NAME])
            if result.returncode:
                return result.returncode

            result = builder.runi(['tar', '-x', '-f', 'fs.tar', '-C', builder.ROOTFS_DIR])
            if result.returncode:
                return result.returncode

            result = builder.runi(['docker', 'plugin', 'create', image, builder.BUILD_DIR])
            if result.returncode:
                return result.returncode

            result = builder.runi(['docker', 'plugin', 'inspect', image])
            if result.returncode:
                return result.returncode

            if releasing:
                result = builder.runi(['docker', 'plugin', 'create', image_latest, builder.BUILD_DIR])
                if result.returncode:
                    return result.returncode

                result = builder.runi(['docker', 'plugin', 'inspect', image_latest])
                if result.returncode:
                    return result.returncode

            if args.no_push:
                return

            result = builder.runi(['docker', 'login', '-u', REPO], tty=True)
            if result.returncode:
                return result.returncode

            result = builder.runi(['docker', 'plugin', 'push', image])
            if result.returncode:
                return result.returncode

            if releasing:
                result = builder.runi(['docker', 'plugin', 'push', image_latest])
                if result.returncode:
                    return result.returncode


if __name__ == '__main__':
    sys.exit(main())
