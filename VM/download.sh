#!/bin/bash

# Judge VM bootstrapper [download part]
#
# Create a VM for judging programming contests
# using Tiny Core Linux with some icing on top...
# 
# You need:
# - wget.

mirror="http://l4u-00.jinr.ru/LinuxArchive/Ftp/tinycorelinux/"
dir="4.x/x86/release/distribution_files/"

function download() {
    for file in $*; do
        rm -f ${file} && wget ${mirror}${dir}${file}
    done
}

download \
    core.gz \
    vmlinuz
