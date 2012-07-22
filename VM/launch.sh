#!/bin/bash

tceimg=$1
if [ -z $tceimg ]; then
   tceimg='tce.img'
fi
shift

kvm -kernel vmlinuz \
    -initrd initrd.gz \
    -append "tce=vda kmap=qwerty/es vga=788 nodhcp" \
    -drive file=${tceimg},if=virtio \
    -serial stdio \
    -net none \
    $*
