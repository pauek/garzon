#!/bin/bash

# Judge VM bootstrapper
#
# Create a VM for judging programming contests
# using Tiny Core Linux with some icing on top...
# 
# You need:
# - to be in the 'sudoers' group.
# - wget.
# - Debian packages 'qemu' + 'qemu-kvm'
# - cpio (already present in most distributions).
#

mirror="http://l4u-00.jinr.ru/LinuxArchive/Ftp/tinycorelinux/"
dir="4.x/x86/release/distribution_files/"
x86_64=''
# x86_64='64'

function download_kernel_and_initrd() {
    rm -f core.gz && wget -O core.gz ${mirror}${dir}core${x86_64}.gz
    rm -f vmlinuz && wget -O vmlinuz ${mirror}${dir}vmlinuz${x86_64}
}

function create_shared_disk() {
    qemu-img create -f qcow2 shared.img 50M
}

function remaster_initrd() {
    go install garzon/driver
    sudo rm -rf initrd
    mkdir -p initrd
    pushd initrd

    # unpack
    zcat ../core.gz | sudo cpio -i -H newc -d 

    # introduce modifications 
    sudo cp $(which driver) usr/bin/driver
    sudo sh -c "cat >> etc/inittab" <<EOF
# garzon
ttyS0::once:/bin/sh
EOF
    sudo sh -c "cat >> opt/bootlocal.sh" <<EOF
# call init script in apps partition
source /mnt/vda/tce/bootlocal.sh 
EOF

    # repack
    sudo find | sudo cpio -o -H newc | gzip -2 > ../initrd.gz
    popd
    sudo rm -rf initrd
}

download_kernel_and_initrd
create_shared_disk
remaster_initrd
