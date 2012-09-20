#!/bin/bash

# Judge VM bootstrapper [remaster part]
#
# Create a VM for judging programming contests
# using Tiny Core Linux with some icing on top...
# 
# You need:
# - to be in the 'sudoers' group.
# - Debian packages 'qemu' + 'qemu-kvm'
# - cpio (already present in most distributions).

function remaster_initrd() {
# Unpack
    sudo rm -rf initrd
    mkdir -p initrd
    pushd initrd
    zcat ../core.gz | sudo cpio -i -H newc -d 

# Introduce modifications 

    # create mountpoint for iso
    sudo mkdir -p mnt/cdrom

    # WARNING: si pones una '/' delante
    # de todos los paths tipo 'cat > path' te cargas tu Linux!!

    # enable root terminal on serial port
    sudo sh -c "cat >> etc/inittab" <<EOF
# garzon
ttyS0::once:/bin/sh
EOF

    # permit execution of script from 'vda' partition
    sudo sh -c "cat >> opt/bootlocal.sh" <<EOF
# call init script in apps partition
source /mnt/vda/tce/bootlocal.sh 
EOF

    # create user 'garzon'
    sudo sh -c "cat >> etc/passwd" <<EOF
garzon:x:5000:5000:Garzon,,,:/home/garzon:/bin/sh
EOF
    sudo sh -c "cat >> etc/group" <<EOF
garzon:x:5000:
EOF
    sudo mkdir -p home/garzon
    sudo chown 5000:5000 home/garzon
    sudo chmod 700 home/garzon
   
# Repack
    sudo find | sudo cpio -o -H newc | gzip -2 > ../initrd.gz
    popd
    sudo rm -rf initrd
}

remaster_initrd
